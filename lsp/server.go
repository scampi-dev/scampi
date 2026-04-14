// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"go.uber.org/zap"

	"scampi.dev/scampi/lang/check"
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

	ctx, conn, client := protocol.NewServer(ctx, s, stream, logger)
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
				Change:    protocol.TextDocumentSyncKindFull,
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
	s.log.Printf("diagnosing workspace: %s", s.rootDir)
	_ = filepath.WalkDir(s.rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".scampi" {
			return nil
		}
		s.diagnoseFile(ctx, path)
		return nil
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
	text := params.ContentChanges[len(params.ContentChanges)-1].Text
	s.log.Printf("didChange: %s (v%d, %d bytes)", td.URI, td.Version, len(text))
	s.docs.Change(td.URI, text, td.Version)
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

func (s *Server) CodeAction(context.Context, *protocol.CodeActionParams) ([]protocol.CodeAction, error) {
	return nil, nil
}
func (s *Server) CodeLens(context.Context, *protocol.CodeLensParams) ([]protocol.CodeLens, error) {
	return nil, nil
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
	context.Context,
	*protocol.DocumentHighlightParams,
) ([]protocol.DocumentHighlight, error) {
	return nil, nil
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
	context.Context,
	*protocol.DocumentFormattingParams,
) ([]protocol.TextEdit, error) {
	return nil, nil
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
func (s *Server) PrepareRename(context.Context, *protocol.PrepareRenameParams) (*protocol.Range, error) {
	return nil, nil
}
func (s *Server) RangeFormatting(
	context.Context,
	*protocol.DocumentRangeFormattingParams,
) ([]protocol.TextEdit, error) {
	return nil, nil
}
func (s *Server) Rename(context.Context, *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	return nil, nil
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
