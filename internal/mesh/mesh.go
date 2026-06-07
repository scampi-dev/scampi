// SPDX-License-Identifier: GPL-3.0-only

package mesh

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/memberlist"

	"scampi.dev/scampi/internal/diag"
)

// Config for Run. SnapshotPath="" disables persistence;
// SnapshotDebounce=0 writes per event without coalescing.
type Config struct {
	Name          string
	BindAddr      string
	BindPort      int
	AdvertiseAddr string
	AdvertisePort int
	Join          []string
	SnapshotPath  string
	Logger        diag.Logger

	ProbeInterval       time.Duration
	SuspectMult         int
	DeadNodeReclaimTime time.Duration
	SnapshotDebounce    time.Duration
}

const defaultDeadNodeReclaimTime = 1 * time.Second

type Peer struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
}

type EventKind int

const (
	EventJoin EventKind = iota + 1
	EventLeave
	EventUpdate
)

func (k EventKind) String() string {
	switch k {
	case EventJoin:
		return "join"
	case EventLeave:
		return "leave"
	case EventUpdate:
		return "update"
	}
	return "unknown"
}

type Event struct {
	Kind EventKind
	Peer Peer
}

type Mesh struct {
	cfg    Config
	list   *memberlist.Memberlist
	self   Peer
	log    diag.Logger
	events chan Event

	ready  atomic.Bool
	closed atomic.Bool

	mu        sync.Mutex
	snapTimer *time.Timer

	// flushWorker writes off the memberlist callback goroutine so
	// Members() doesn't re-enter the nodeLock NotifyJoin holds.
	flushReq  chan struct{}
	flushQuit chan struct{}
	flushDone chan struct{}

	closeOnce sync.Once
}

// Run brings up memberlist. Join failures are logged, not
// returned; the caller decides whether unavailability is fatal.
func Run(ctx context.Context, cfg Config) (*Mesh, error) {
	if cfg.Logger == nil {
		cfg.Logger = diag.Discard{}
	}

	mlc := memberlist.DefaultLANConfig()
	mlc.Name = cfg.Name
	mlc.BindAddr = cfg.BindAddr
	mlc.BindPort = cfg.BindPort
	if cfg.AdvertiseAddr != "" {
		mlc.AdvertiseAddr = cfg.AdvertiseAddr
		mlc.AdvertisePort = cfg.AdvertisePort
	}
	if cfg.ProbeInterval > 0 {
		mlc.ProbeInterval = cfg.ProbeInterval
	}
	if cfg.SuspectMult > 0 {
		mlc.SuspicionMult = cfg.SuspectMult
	}
	mlc.DeadNodeReclaimTime = cfg.DeadNodeReclaimTime
	if mlc.DeadNodeReclaimTime == 0 {
		mlc.DeadNodeReclaimTime = defaultDeadNodeReclaimTime
	}
	mlc.Logger = stdlog.New(diagWriter{cfg.Logger}, "", 0)

	m := &Mesh{
		cfg:       cfg,
		log:       cfg.Logger,
		events:    make(chan Event, 64),
		flushReq:  make(chan struct{}, 1),
		flushQuit: make(chan struct{}),
		flushDone: make(chan struct{}),
	}
	mlc.Events = &eventBridge{m: m}
	mlc.Conflict = &conflictBridge{m: m}

	list, err := memberlist.Create(mlc)
	if err != nil {
		return nil, fmt.Errorf("mesh create: %w", err)
	}
	m.list = list
	m.self = nodeToPeer(list.LocalNode())
	m.ready.Store(true)
	go m.flushWorker()

	candidates := append([]string(nil), cfg.Join...)
	if cfg.SnapshotPath != "" {
		if snap, rerr := ReadSnapshot(cfg.SnapshotPath); rerr == nil {
			for _, p := range snap.Peers {
				if p.Name == cfg.Name {
					continue
				}
				candidates = append(candidates, p.Addr)
			}
		} else if !errors.Is(rerr, os.ErrNotExist) {
			m.log.Warn(ctx, "mesh snapshot read failed", "err", rerr)
		}
	}

	if len(candidates) > 0 {
		if _, jerr := list.Join(candidates); jerr != nil {
			m.log.Warn(
				ctx, "mesh join: no candidates reachable, retrying",
				"next", joinRetryInitial, "err", jerr,
			)
			go m.joinRetry(ctx, candidates)
		}
	}

	m.markDirty()
	return m, nil
}

const (
	joinRetryInitial = 500 * time.Millisecond
	joinRetryMax     = 30 * time.Second
)

// joinRetry keeps re-attempting list.Join with exponential backoff
// until any candidate accepts. Without this a peer that boots
// before its seed stays solo forever, because memberlist's Join is
// one-shot and #1 (the seed) has no return path to #2 (the joiner)
// unless gossip already linked them.
func (m *Mesh) joinRetry(ctx context.Context, candidates []string) {
	delay := joinRetryInitial
	for attempt := 1; ; attempt++ {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		if m.closed.Load() {
			return
		}
		next := min(delay*2, joinRetryMax)
		m.log.Debug(ctx, "mesh-join retrying", "attempt", attempt, "next", next)
		if _, err := m.list.Join(candidates); err == nil {
			m.log.Info(ctx, "mesh join succeeded after retry", "attempt", attempt)
			return
		}
		delay = next
	}
}

// Members returns alive peers; the slice includes self.
func (m *Mesh) Members() []Peer {
	nodes := m.list.Members()
	out := make([]Peer, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, nodeToPeer(n))
	}
	return out
}

func (m *Mesh) Self() Peer { return m.self }

// Events is buffered; slow consumers see drops logged via Logger.
func (m *Mesh) Events() <-chan Event { return m.events }

// Leave gossips departure so peers skip the SWIM suspect window.
func (m *Mesh) Leave(timeout time.Duration) error {
	return m.list.Leave(timeout)
}

// Shutdown is idempotent.
func (m *Mesh) Shutdown() error {
	var err error
	m.closeOnce.Do(func() {
		m.closed.Store(true)

		m.mu.Lock()
		if m.snapTimer != nil {
			m.snapTimer.Stop()
			m.snapTimer = nil
		}
		m.mu.Unlock()

		close(m.flushQuit)
		<-m.flushDone

		if m.cfg.SnapshotPath != "" {
			m.doFlushSnapshot()
		}

		err = m.list.Shutdown()
		close(m.events)
	})
	return err
}

func (m *Mesh) Healthy() bool {
	return !m.closed.Load() && m.list != nil
}

func (m *Mesh) markDirty() {
	if m.cfg.SnapshotPath == "" || m.closed.Load() {
		return
	}
	if m.cfg.SnapshotDebounce == 0 {
		m.queueFlush()
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.snapTimer == nil {
		m.snapTimer = time.AfterFunc(m.cfg.SnapshotDebounce, m.queueFlush)
	} else {
		m.snapTimer.Reset(m.cfg.SnapshotDebounce)
	}
}

func (m *Mesh) queueFlush() {
	select {
	case m.flushReq <- struct{}{}:
	default:
	}
}

func (m *Mesh) flushWorker() {
	defer close(m.flushDone)
	for {
		select {
		case <-m.flushQuit:
			return
		case <-m.flushReq:
			if m.cfg.SnapshotPath != "" {
				m.doFlushSnapshot()
			}
		}
	}
}

func (m *Mesh) doFlushSnapshot() {
	// Members() drops self post-Leave; the snapshot is both a
	// rejoin seed list and a peer-display record, so self belongs
	// in it. dedupePeers handles the duplicate when Members()
	// still includes self.
	peers := append(m.Members(), m.self)
	s := &Snapshot{Self: m.self.Name, Peers: peers}
	if err := writeSnapshot(m.cfg.SnapshotPath, s); err != nil {
		m.log.Warn(context.Background(), "mesh snapshot write failed", "err", err)
	}
}

func (m *Mesh) onEvent(kind EventKind, p Peer) {
	if !m.ready.Load() || m.closed.Load() {
		return
	}
	select {
	case m.events <- Event{Kind: kind, Peer: p}:
	default:
		m.log.Warn(
			context.Background(),
			"mesh event dropped",
			"kind",
			kind.String(),
			"peer",
			p.Name,
		)
	}
	m.markDirty()
}

func nodeToPeer(n *memberlist.Node) Peer {
	return Peer{
		Name: n.Name,
		Addr: net.JoinHostPort(n.Addr.String(), strconv.Itoa(int(n.Port))),
	}
}

type eventBridge struct {
	m *Mesh
}

func (e *eventBridge) NotifyJoin(n *memberlist.Node)   { e.m.onEvent(EventJoin, nodeToPeer(n)) }
func (e *eventBridge) NotifyLeave(n *memberlist.Node)  { e.m.onEvent(EventLeave, nodeToPeer(n)) }
func (e *eventBridge) NotifyUpdate(n *memberlist.Node) { e.m.onEvent(EventUpdate, nodeToPeer(n)) }

type conflictBridge struct {
	m *Mesh
}

func (c *conflictBridge) NotifyConflict(existing, other *memberlist.Node) {
	c.m.log.Error(
		context.Background(), "mesh name conflict",
		"name", existing.Name,
		"existing", nodeToPeer(existing).Addr,
		"other", nodeToPeer(other).Addr,
	)
}

// diagWriter routes memberlist's "[LEVEL] memberlist: msg" lines
// onto matching diag.Logger calls with the prefix stripped.
type diagWriter struct {
	l diag.Logger
}

func (w diagWriter) Write(p []byte) (int, error) {
	raw := strings.TrimSpace(string(p))
	ctx := context.Background()
	msg := stripMemberlistPrefix(raw)
	switch {
	case strings.HasPrefix(raw, "[ERR]"):
		w.l.Error(ctx, msg, "src", "memberlist")
	case strings.HasPrefix(raw, "[WARN]"):
		w.l.Warn(ctx, msg, "src", "memberlist")
	case strings.HasPrefix(raw, "[INFO]"):
		w.l.Info(ctx, msg, "src", "memberlist")
	default:
		w.l.Debug(ctx, msg, "src", "memberlist")
	}
	return len(p), nil
}

func stripMemberlistPrefix(s string) string {
	if _, after, ok := strings.Cut(s, "]"); ok {
		return strings.TrimSpace(strings.TrimPrefix(after, " memberlist:"))
	}
	return s
}
