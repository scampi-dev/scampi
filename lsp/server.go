// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"go.uber.org/zap"

	"scampi.dev/scampi/lang/check"
	"scampi.dev/scampi/lang/format"
	"scampi.dev/scampi/mod"
)

// Server implements the LSP protocol for scampi configuration files.
type Server struct {
	catalog  *Catalog
	modules  map[string]*check.Scope
	stubDefs *StubDefs
	docs     *Documents
	client   protocol.Client
	conn     jsonrpc2.Conn
	log      *log.Logger

	rootDir  string
	module   *mod.Module
	cacheDir string
}

// Version is set at build time via ldflags.
var Version = "v0.0.0-dev"

const (
	diagSourceParser = "scampi"
	diagSourceLSP    = "scampls"
)

// Option configures the LSP server.
type Option func(*Server)

// WithLog sets a logger for diagnostic output.
func WithLog(l *log.Logger) Option {
	return func(s *Server) { s.log = l }
}

// Serve starts the LSP server over the given reader/writer pair.
// It blocks until the connection is closed or the context is cancelled.
func Serve(ctx context.Context, in io.Reader, out io.Writer, opts ...Option) error {
	rwc := &readWriteCloser{in: in, out: out}
	stream := jsonrpc2.NewStream(rwc)
	logger := zap.NewNop()

	s := &Server{
		catalog:  NewCatalog(),
		modules:  bootstrapModules(),
		stubDefs: NewStubDefs(),
		docs:     NewDocuments(),
		log:      log.New(io.Discard, "", 0),
	}
	for _, opt := range opts {
		opt(s)
	}

	s.log.Printf("server starting, %d stdlib entries loaded", len(s.catalog.Names()))

	conn := jsonrpc2.NewConn(stream)
	client := protocol.ClientDispatcher(conn, logger.Named("client"))
	ctx = protocol.WithClient(ctx, client)
	conn.Go(ctx,
		protocol.Handlers(
			s.inlayHintHandler(
				protocol.ServerHandler(s, jsonrpc2.MethodNotFoundHandler),
			),
		),
	)
	s.client = client
	s.conn = conn

	s.log.Printf("connection established, waiting for initialize")

	select {
	case <-ctx.Done():
		s.log.Printf("context done: %v", ctx.Err())
		return ctx.Err()
	case <-conn.Done():
		s.log.Printf("connection done: %v", conn.Err())
		return conn.Err()
	}
}

// readWriteCloser adapts separate reader/writer into io.ReadWriteCloser
// for jsonrpc2.NewStream.
type readWriteCloser struct {
	in  io.Reader
	out io.Writer
}

func (rwc *readWriteCloser) Read(p []byte) (int, error)  { return rwc.in.Read(p) }
func (rwc *readWriteCloser) Write(p []byte) (int, error) { return rwc.out.Write(p) }
func (rwc *readWriteCloser) Close() error                { return nil }

// Lifecycle
// -----------------------------------------------------------------------------

func (s *Server) Initialize(
	_ context.Context,
	params *protocol.InitializeParams,
) (*protocol.InitializeResult, error) {
	clientName := ""
	if params.ClientInfo != nil {
		clientName = params.ClientInfo.Name
	}
	rootURI := ""
	if len(params.WorkspaceFolders) > 0 {
		rootURI = string(params.WorkspaceFolders[0].URI)
		s.rootDir = uri.URI(protocol.DocumentURI(rootURI)).Filename()
	}
	s.log.Printf("initialize: client=%q root=%q", clientName, rootURI)
	s.loadModule()
	s.loadUserModules()
	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    protocol.TextDocumentSyncKindIncremental,
				Save: &protocol.SaveOptions{
					IncludeText: true,
				},
			},
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{"(", ".", ",", "\""},
			},
			SignatureHelpProvider: &protocol.SignatureHelpOptions{
				TriggerCharacters: []string{"(", ","},
			},
			HoverProvider:           &protocol.HoverOptions{},
			DefinitionProvider:      &protocol.DefinitionOptions{},
			ReferencesProvider:      &protocol.ReferenceOptions{},
			DocumentSymbolProvider:  &protocol.DocumentSymbolOptions{},
			WorkspaceSymbolProvider: &protocol.WorkspaceSymbolOptions{},
			RenameProvider: &protocol.RenameOptions{
				PrepareProvider: true,
			},
			DocumentFormattingProvider: &protocol.DocumentFormattingOptions{},
			DocumentHighlightProvider:  &protocol.DocumentHighlightOptions{},
			CodeLensProvider:           &protocol.CodeLensOptions{},
			CodeActionProvider: &protocol.CodeActionOptions{
				CodeActionKinds: []protocol.CodeActionKind{
					protocol.QuickFix,
				},
			},
		},
		ServerInfo: &protocol.ServerInfo{
			Name:    "scampls",
			Version: Version,
		},
	}, nil
}

func (s *Server) loadModule() {
	if s.rootDir == "" {
		return
	}
	modPath := filepath.Join(s.rootDir, "scampi.mod")
	data, err := os.ReadFile(modPath)
	if err != nil {
		s.log.Printf("no scampi.mod at %s: %v", modPath, err)
		return
	}
	m, err := mod.Parse(modPath, data)
	if err != nil {
		s.log.Printf("scampi.mod parse error: %v", err)
		return
	}
	s.module = m
	s.cacheDir = mod.DefaultCacheDir()
	s.log.Printf("loaded module %s (%d deps)", m.Module, len(m.Require))
}

func (s *Server) Initialized(ctx context.Context, _ *protocol.InitializedParams) error {
	s.log.Printf("initialized")

	// Watch scampi.mod for changes so we can reload deps without restart.
	_ = s.client.RegisterCapability(ctx, &protocol.RegistrationParams{
		Registrations: []protocol.Registration{{
			ID:     "watch-scampi-mod",
			Method: "workspace/didChangeWatchedFiles",
			RegisterOptions: protocol.DidChangeWatchedFilesRegistrationOptions{
				Watchers: []protocol.FileSystemWatcher{{
					GlobPattern: "**/scampi.mod",
					Kind:        protocol.WatchKindCreate + protocol.WatchKindChange + protocol.WatchKindDelete,
				}},
			},
		}},
	})

	go s.diagnoseWorkspace(ctx)
	return nil
}

func (s *Server) diagnoseWorkspace(ctx context.Context) {
	if s.rootDir == "" {
		return
	}

	// Collect files first so we can report progress.
	var files []string
	_ = filepath.WalkDir(s.rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".scampi" {
			return nil
		}
		files = append(files, path)
		return nil
	})

	if len(files) == 0 {
		return
	}

	s.log.Printf("diagnosing workspace: %s (%d files)", s.rootDir, len(files))

	if s.client == nil {
		for _, path := range files {
			s.diagnoseFile(ctx, path)
		}
		return
	}

	token := protocol.NewProgressToken("workspace-scan")
	_ = s.client.WorkDoneProgressCreate(ctx, &protocol.WorkDoneProgressCreateParams{
		Token: *token,
	})
	_ = s.client.Progress(ctx, &protocol.ProgressParams{
		Token: *token,
		Value: protocol.WorkDoneProgressBegin{
			Kind:    protocol.WorkDoneProgressKindBegin,
			Title:   "Scanning workspace",
			Message: fmt.Sprintf("0/%d files", len(files)),
		},
	})

	for i, path := range files {
		s.diagnoseFile(ctx, path)
		_ = s.client.Progress(ctx, &protocol.ProgressParams{
			Token: *token,
			Value: protocol.WorkDoneProgressReport{
				Kind:       protocol.WorkDoneProgressKindReport,
				Message:    fmt.Sprintf("%d/%d files", i+1, len(files)),
				Percentage: uint32(float64(i+1) / float64(len(files)) * 100),
			},
		})
	}

	_ = s.client.Progress(ctx, &protocol.ProgressParams{
		Token: *token,
		Value: protocol.WorkDoneProgressEnd{
			Kind:    protocol.WorkDoneProgressKindEnd,
			Message: fmt.Sprintf("Done — %d files", len(files)),
		},
	})
}
func (s *Server) Shutdown(context.Context) error {
	s.log.Printf("shutdown")
	return nil
}

func (s *Server) Exit(_ context.Context) error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// Document synchronization
// -----------------------------------------------------------------------------

func (s *Server) DidOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {
	td := params.TextDocument
	s.log.Printf("didOpen: %s (v%d, %d bytes)", td.URI, td.Version, len(td.Text))
	s.docs.Open(td.URI, td.Text, td.Version)
	return s.publishDiagnostics(ctx, td.URI, td.Text)
}

func (s *Server) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	td := params.TextDocument
	if len(params.ContentChanges) == 0 {
		return nil
	}
	text := s.docs.ApplyIncremental(td.URI, params.ContentChanges, td.Version)
	s.log.Printf("didChange: %s (v%d, %d changes, %d bytes)",
		td.URI, td.Version, len(params.ContentChanges), len(text))
	return s.publishDiagnostics(ctx, td.URI, text)
}

func (s *Server) DidSave(ctx context.Context, params *protocol.DidSaveTextDocumentParams) error {
	s.log.Printf("didSave: %s", params.TextDocument.URI)
	if params.Text != "" {
		s.docs.Change(params.TextDocument.URI, params.Text, 0)
		return s.publishDiagnostics(ctx, params.TextDocument.URI, params.Text)
	}
	return nil
}

func (s *Server) DidClose(_ context.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.log.Printf("didClose: %s", params.TextDocument.URI)
	s.docs.Close(params.TextDocument.URI)
	return nil
}

func (s *Server) publishDiagnostics(ctx context.Context, uri protocol.DocumentURI, text string) error {
	diags := s.evaluate(ctx, uri, text)
	if diags == nil {
		diags = []protocol.Diagnostic{}
	}
	s.log.Printf("publishDiagnostics: %s → %d diagnostics", uri, len(diags))
	for _, d := range diags {
		s.log.Printf("  diag: L%d:%d %s", d.Range.Start.Line, d.Range.Start.Character, d.Message)
	}
	return s.client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diags,
	})
}

// Unimplemented methods
// -----------------------------------------------------------------------------
//
//nolint:revive // Stubs satisfy protocol.Server — unused params are intentional.

func (s *Server) WorkDoneProgressCancel(context.Context, *protocol.WorkDoneProgressCancelParams) error {
	return nil
}
func (s *Server) LogTrace(context.Context, *protocol.LogTraceParams) error { return nil }
func (s *Server) SetTrace(context.Context, *protocol.SetTraceParams) error { return nil }

func (s *Server) CodeAction(ctx context.Context, params *protocol.CodeActionParams) ([]protocol.CodeAction, error) {
	return s.codeAction(ctx, params)
}
func (s *Server) CodeLens(_ context.Context, params *protocol.CodeLensParams) ([]protocol.CodeLens, error) {
	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	filePath := uriToPath(params.TextDocument.URI)
	f, _ := Parse(filePath, []byte(doc.Content))
	if f == nil {
		return nil, nil
	}

	data := []byte(doc.Content)
	var lenses []protocol.CodeLens

	for _, d := range f.Decls {
		name, span := declNameAndSpan(d)
		if name == "" {
			continue
		}
		refs := findIdents(f, filePath, data, name)
		// Subtract 1 for the definition itself.
		count := len(refs) - 1
		if count < 0 {
			count = 0
		}
		label := fmt.Sprintf("%d references", count)
		if count == 1 {
			label = "1 reference"
		}
		r := tokenSpanToRange(data, span)
		lenses = append(lenses, protocol.CodeLens{
			Range: r,
			Command: &protocol.Command{
				Title:   label,
				Command: "editor.action.showReferences",
				Arguments: []any{
					params.TextDocument.URI,
					r.Start,
					refs,
				},
			},
		})
	}

	s.log.Printf("codeLens: %s → %d lenses", filePath, len(lenses))
	return lenses, nil
}
func (s *Server) CodeLensResolve(context.Context, *protocol.CodeLens) (*protocol.CodeLens, error) {
	return nil, nil
}
func (s *Server) CompletionResolve(
	_ context.Context,
	item *protocol.CompletionItem,
) (*protocol.CompletionItem, error) {
	return item, nil
}
func (s *Server) Declaration(context.Context, *protocol.DeclarationParams) ([]protocol.Location, error) {
	return nil, nil
}
func (s *Server) DidChangeConfiguration(context.Context, *protocol.DidChangeConfigurationParams) error {
	return nil
}
func (s *Server) DidChangeWatchedFiles(ctx context.Context, params *protocol.DidChangeWatchedFilesParams) error {
	for _, change := range params.Changes {
		path := uri.URI(change.URI).Filename()
		if filepath.Base(path) == "scampi.mod" {
			s.log.Printf("scampi.mod changed, reloading modules")
			s.modules = bootstrapModules()
			s.loadModule()
			s.loadUserModules()
			go s.rediagnoseOpenFiles(ctx)
			return nil
		}
	}
	return nil
}

func (s *Server) rediagnoseOpenFiles(ctx context.Context) {
	for _, doc := range s.docs.All() {
		_ = s.publishDiagnostics(ctx, doc.URI, doc.Content)
	}
}
func (s *Server) DidChangeWorkspaceFolders(
	context.Context,
	*protocol.DidChangeWorkspaceFoldersParams,
) error {
	return nil
}
func (s *Server) DocumentColor(
	context.Context,
	*protocol.DocumentColorParams,
) ([]protocol.ColorInformation, error) {
	return nil, nil
}
func (s *Server) ColorPresentation(
	context.Context,
	*protocol.ColorPresentationParams,
) ([]protocol.ColorPresentation, error) {
	return nil, nil
}
func (s *Server) DocumentHighlight(
	_ context.Context,
	params *protocol.DocumentHighlightParams,
) ([]protocol.DocumentHighlight, error) {
	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	word := wordAtOffset(doc.Content, offsetFromPosition(
		doc.Content,
		params.Position.Line,
		params.Position.Character,
	))
	if word == "" {
		return nil, nil
	}

	filePath := uriToPath(params.TextDocument.URI)
	f, _ := Parse(filePath, []byte(doc.Content))
	if f == nil {
		return nil, nil
	}

	data := []byte(doc.Content)
	var locs []protocol.Location
	if strings.Contains(word, ".") {
		locs = findDottedRefs(f, filePath, data, word)
	} else {
		locs = findIdents(f, filePath, data, word)
	}

	// Find definition to mark it as Write, all others as Read.
	defSpan := findDefinition(f, word)

	var highlights []protocol.DocumentHighlight
	for _, loc := range locs {
		kind := protocol.DocumentHighlightKindRead
		if defSpan != nil {
			r := tokenSpanToRange(data, *defSpan)
			if r.Start == loc.Range.Start {
				kind = protocol.DocumentHighlightKindWrite
			}
		}
		highlights = append(highlights, protocol.DocumentHighlight{
			Range: loc.Range,
			Kind:  kind,
		})
	}
	s.log.Printf("documentHighlight: %s %q → %d highlights", filePath, word, len(highlights))
	return highlights, nil
}
func (s *Server) DocumentLink(context.Context, *protocol.DocumentLinkParams) ([]protocol.DocumentLink, error) {
	return nil, nil
}
func (s *Server) DocumentLinkResolve(context.Context, *protocol.DocumentLink) (*protocol.DocumentLink, error) {
	return nil, nil
}
func (s *Server) ExecuteCommand(context.Context, *protocol.ExecuteCommandParams) (any, error) {
	return nil, nil
}
func (s *Server) FoldingRanges(
	context.Context,
	*protocol.FoldingRangeParams,
) ([]protocol.FoldingRange, error) {
	return nil, nil
}
func (s *Server) Formatting(
	_ context.Context,
	params *protocol.DocumentFormattingParams,
) ([]protocol.TextEdit, error) {
	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	formatted, err := format.Format([]byte(doc.Content))
	if err != nil {
		s.log.Printf("formatting: %s error: %v", params.TextDocument.URI, err)
		return nil, nil
	}

	if string(formatted) == doc.Content {
		s.log.Printf("formatting: %s (no changes)", params.TextDocument.URI)
		return nil, nil
	}
	s.log.Printf("formatting: %s (changed)", params.TextDocument.URI)

	lines := strings.Count(doc.Content, "\n")
	return []protocol.TextEdit{{
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: uint32(lines + 1), Character: 0},
		},
		NewText: string(formatted),
	}}, nil
}
func (s *Server) Implementation(context.Context, *protocol.ImplementationParams) ([]protocol.Location, error) {
	return nil, nil
}
func (s *Server) OnTypeFormatting(
	context.Context,
	*protocol.DocumentOnTypeFormattingParams,
) ([]protocol.TextEdit, error) {
	return nil, nil
}
func (s *Server) PrepareRename(ctx context.Context, params *protocol.PrepareRenameParams) (*protocol.Range, error) {
	return s.prepareRename(ctx, params)
}
func (s *Server) RangeFormatting(
	context.Context,
	*protocol.DocumentRangeFormattingParams,
) ([]protocol.TextEdit, error) {
	return nil, nil
}
func (s *Server) Rename(ctx context.Context, params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	return s.rename(ctx, params)
}
func (s *Server) TypeDefinition(
	context.Context,
	*protocol.TypeDefinitionParams,
) ([]protocol.Location, error) {
	return nil, nil
}
func (s *Server) WillSave(context.Context, *protocol.WillSaveTextDocumentParams) error { return nil }
func (s *Server) WillSaveWaitUntil(
	context.Context,
	*protocol.WillSaveTextDocumentParams,
) ([]protocol.TextEdit, error) {
	return nil, nil
}
func (s *Server) ShowDocument(
	context.Context,
	*protocol.ShowDocumentParams,
) (*protocol.ShowDocumentResult, error) {
	return nil, nil
}
func (s *Server) WillCreateFiles(
	context.Context,
	*protocol.CreateFilesParams,
) (*protocol.WorkspaceEdit, error) {
	return nil, nil
}
func (s *Server) DidCreateFiles(context.Context, *protocol.CreateFilesParams) error { return nil }
func (s *Server) DidRenameFiles(context.Context, *protocol.RenameFilesParams) error { return nil }
func (s *Server) DidDeleteFiles(context.Context, *protocol.DeleteFilesParams) error { return nil }
func (s *Server) CodeLensRefresh(context.Context) error                             { return nil }
func (s *Server) SemanticTokensRefresh(context.Context) error                       { return nil }
func (s *Server) WillRenameFiles(
	context.Context,
	*protocol.RenameFilesParams,
) (*protocol.WorkspaceEdit, error) {
	return nil, nil
}
func (s *Server) WillDeleteFiles(
	context.Context,
	*protocol.DeleteFilesParams,
) (*protocol.WorkspaceEdit, error) {
	return nil, nil
}
func (s *Server) PrepareCallHierarchy(
	context.Context,
	*protocol.CallHierarchyPrepareParams,
) ([]protocol.CallHierarchyItem, error) {
	return nil, nil
}
func (s *Server) IncomingCalls(
	context.Context,
	*protocol.CallHierarchyIncomingCallsParams,
) ([]protocol.CallHierarchyIncomingCall, error) {
	return nil, nil
}
func (s *Server) OutgoingCalls(
	context.Context,
	*protocol.CallHierarchyOutgoingCallsParams,
) ([]protocol.CallHierarchyOutgoingCall, error) {
	return nil, nil
}
func (s *Server) SemanticTokensFull(
	context.Context,
	*protocol.SemanticTokensParams,
) (*protocol.SemanticTokens, error) {
	return nil, nil
}
func (s *Server) SemanticTokensFullDelta(
	context.Context,
	*protocol.SemanticTokensDeltaParams,
) (any, error) {
	return nil, nil
}
func (s *Server) SemanticTokensRange(
	context.Context,
	*protocol.SemanticTokensRangeParams,
) (*protocol.SemanticTokens, error) {
	return nil, nil
}
func (s *Server) LinkedEditingRange(
	context.Context,
	*protocol.LinkedEditingRangeParams,
) (*protocol.LinkedEditingRanges, error) {
	return nil, nil
}
func (s *Server) Moniker(context.Context, *protocol.MonikerParams) ([]protocol.Moniker, error) {
	return nil, nil
}
func (s *Server) Request(context.Context, string, any) (any, error) {
	return nil, nil
}
