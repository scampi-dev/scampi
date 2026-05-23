// SPDX-License-Identifier: GPL-3.0-only

// Package identity caches per-target identity lookups: name<->uid,
// name<->gid, and "does this user/group exist?" answers.
//
// These facts derive from /etc/passwd, /etc/group and the like. They
// are constants for a target's lifetime UNLESS scampi itself mutates
// them via step.user or step.group. The target's CreateUser /
// ModifyUser / DeleteUser (and group equivalents) call Invalidate*
// after a successful mutation so the cache stays honest in the
// namespace scampi manages.
//
// Negative results are cached too: a lookup that resolved to "user
// doesn't exist" is just as expensive as one that succeeded, and the
// same answer is often asked repeatedly during drift detection.
//
// The cache is safe for concurrent use.
package identity

import "sync"

type (
	// Cache memoizes identity lookups for a single target.
	Cache struct {
		mu       sync.RWMutex
		users    map[string]userEntry
		groups   map[string]groupEntry
		userByID map[int]string
		grpByID  map[int]string
	}

	userEntry struct {
		uid    int
		exists bool
	}
	groupEntry struct {
		gid    int
		exists bool
	}
)

// New returns an empty cache.
func New() *Cache {
	return &Cache{
		users:    make(map[string]userEntry),
		groups:   make(map[string]groupEntry),
		userByID: make(map[int]string),
		grpByID:  make(map[int]string),
	}
}

// UID returns the cached uid for name. ok=false means the cache has no
// entry; ok=true with exists=false means we previously looked up name
// and learned no such user exists.
func (c *Cache) UID(name string) (uid int, exists, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, hit := c.users[name]
	if !hit {
		return 0, false, false
	}
	return e.uid, e.exists, true
}

// GID is the group counterpart of UID.
func (c *Cache) GID(name string) (gid int, exists, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, hit := c.groups[name]
	if !hit {
		return 0, false, false
	}
	return e.gid, e.exists, true
}

// UserName returns the cached name for uid.
func (c *Cache) UserName(uid int) (name string, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	name, ok = c.userByID[uid]
	return
}

// GroupName returns the cached name for gid.
func (c *Cache) GroupName(gid int) (name string, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	name, ok = c.grpByID[gid]
	return
}

// SetUser caches a known mapping name<->uid.
func (c *Cache) SetUser(name string, uid int) {
	c.mu.Lock()
	c.users[name] = userEntry{uid: uid, exists: true}
	c.userByID[uid] = name
	c.mu.Unlock()
}

// SetGroup caches a known mapping name<->gid.
func (c *Cache) SetGroup(name string, gid int) {
	c.mu.Lock()
	c.groups[name] = groupEntry{gid: gid, exists: true}
	c.grpByID[gid] = name
	c.mu.Unlock()
}

// MarkUserAbsent records that name is known not to exist as a user.
func (c *Cache) MarkUserAbsent(name string) {
	c.mu.Lock()
	c.users[name] = userEntry{}
	c.mu.Unlock()
}

// MarkGroupAbsent records that name is known not to exist as a group.
func (c *Cache) MarkGroupAbsent(name string) {
	c.mu.Lock()
	c.groups[name] = groupEntry{}
	c.mu.Unlock()
}

// InvalidateUser drops cached entries for name in both directions.
// Called from target mutation methods (CreateUser, ModifyUser,
// DeleteUser) on success.
//
// nil receiver is tolerated: a target without an identity cache
// still has callers invoking Invalidate*, and a no-op keeps those
// call sites uniform.
func (c *Cache) InvalidateUser(name string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.users[name]; ok && e.exists {
		delete(c.userByID, e.uid)
	}
	delete(c.users, name)
}

// InvalidateGroup drops cached entries for name in both directions.
func (c *Cache) InvalidateGroup(name string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.groups[name]; ok && e.exists {
		delete(c.grpByID, e.gid)
	}
	delete(c.groups, name)
}
