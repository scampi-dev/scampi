// SPDX-License-Identifier: GPL-3.0-only

package lsp

import (
	"sync"

	"go.lsp.dev/protocol"
)

// Document represents an open file tracked by the LSP server.
type Document struct {
	URI     protocol.DocumentURI
	Content string
	Version int32
}

// Documents is a thread-safe store of open text documents. The server
// keeps one of these for the lifetime of the connection and updates it
// in response to didOpen / didChange / didClose notifications.
type Documents struct {
	mu   sync.RWMutex
	docs map[protocol.DocumentURI]*Document
}

func NewDocuments() *Documents {
	return &Documents{docs: make(map[protocol.DocumentURI]*Document)}
}

func (d *Documents) Open(uri protocol.DocumentURI, content string, version int32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.docs[uri] = &Document{URI: uri, Content: content, Version: version}
}

func (d *Documents) Change(uri protocol.DocumentURI, content string, version int32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if doc, ok := d.docs[uri]; ok {
		doc.Content = content
		doc.Version = version
	}
}

func (d *Documents) Close(uri protocol.DocumentURI) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.docs, uri)
}

func (d *Documents) Get(uri protocol.DocumentURI) (*Document, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	doc, ok := d.docs[uri]
	return doc, ok
}

// All returns a snapshot of every open document.
func (d *Documents) All() []Document {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Document, 0, len(d.docs))
	for _, doc := range d.docs {
		out = append(out, *doc)
	}
	return out
}
