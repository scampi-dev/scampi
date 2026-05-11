// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

var pageTpl = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html><head>
<meta charset="utf-8">
<title>{{.Path}}</title>
<style>
body {
  max-width: 52em; margin: 2em auto; padding: 0 1em;
  font-family: -apple-system, BlinkMacSystemFont, system-ui, sans-serif;
  line-height: 1.55; color: #222;
}
pre { background: #f6f8fa; padding: 0.8em 1em; overflow-x: auto; border-radius: 6px; font-size: 0.9em; }
code { background: #f6f8fa; padding: 0.1em 0.3em; border-radius: 3px; font-size: 0.9em; }
pre code { background: none; padding: 0; }
table { border-collapse: collapse; margin: 1em 0; }
th, td { border: 1px solid #d0d7de; padding: 0.35em 0.7em; }
th { background: #f6f8fa; }
hr { border: 0; border-top: 1px solid #d0d7de; margin: 2em 0; }
a { color: #0969da; text-decoration: none; }
a:hover { text-decoration: underline; }
h1, h2 { border-bottom: 1px solid #eaecef; padding-bottom: 0.3em; }
blockquote { border-left: 4px solid #d0d7de; padding: 0 1em; color: #57606a; margin: 1em 0; }
.nav { margin-bottom: 1.5em; font-size: 0.9em; color: #57606a; font-family: ui-monospace, monospace; }
.dir { list-style: none; padding: 0; }
.dir li { padding: 0.15em 0; }
.dir li::before { content: "📄 "; }
.dir li.d::before { content: "📁 "; }
</style></head><body>
<div class="nav">{{.Nav}}</div>
{{.Body}}
</body></html>`))

func main() {
	dir := flag.String("dir", ".sandbox", "root directory to serve")
	addr := flag.String("addr", ":7080", "listen address")
	flag.Parse()

	abs, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stat(abs); err != nil {
		log.Fatalf("dir not found: %v", err)
	}

	log.Printf("serving %s on http://localhost%s", abs, *addr)
	if err := http.ListenAndServe(*addr, handler(abs)); err != nil {
		log.Fatal(err)
	}
}

func handler(root string) http.HandlerFunc {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
	return func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/")
		p := filepath.Join(root, rel)
		cleaned, err := filepath.Abs(p)
		if err != nil || !strings.HasPrefix(cleaned, root) {
			http.Error(w, "out of bounds", http.StatusBadRequest)
			return
		}

		info, err := os.Stat(cleaned)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		switch {
		case info.IsDir():
			serveDir(w, r, cleaned, rel)
		case strings.HasSuffix(cleaned, ".md"):
			serveMD(w, r, cleaned, rel, md)
		default:
			http.ServeFile(w, r, cleaned)
		}
	}
}

func serveDir(w http.ResponseWriter, r *http.Request, dir, rel string) {
	if rel != "" && !strings.HasSuffix(r.URL.Path, "/") {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})

	var b strings.Builder
	b.WriteString(`<ul class="dir">`)
	if rel != "" {
		b.WriteString(`<li class="d"><a href="../">../</a></li>`)
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			_, _ = fmt.Fprintf(&b, `<li class="d"><a href="%s/">%s/</a></li>`, name, name)
		} else {
			_, _ = fmt.Fprintf(&b, `<li><a href="%s">%s</a></li>`, name, name)
		}
	}
	b.WriteString("</ul>")
	renderPage(w, "/"+rel, template.HTML(b.String()))
}

func serveMD(w http.ResponseWriter, _ *http.Request, p, rel string, md goldmark.Markdown) {
	content, err := os.ReadFile(p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var buf strings.Builder
	if err := md.Convert(content, &buf); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderPage(w, "/"+rel, template.HTML(buf.String()))
}

func renderPage(w http.ResponseWriter, path string, body template.HTML) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTpl.Execute(w, struct {
		Path string
		Nav  template.HTML
		Body template.HTML
	}{
		Path: path,
		Nav:  navLinks(path),
		Body: body,
	})
}

func navLinks(path string) template.HTML {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	var b strings.Builder
	b.WriteString(`<a href="/">/</a>`)
	if path == "/" {
		return template.HTML(b.String())
	}
	var acc strings.Builder
	for i, part := range parts {
		if part == "" {
			continue
		}
		acc.WriteByte('/')
		acc.WriteString(part)
		if i < len(parts)-1 {
			_, _ = fmt.Fprintf(&b, ` <a href="%s/">%s</a> /`, acc.String(), part)
		} else {
			b.WriteString(" " + part)
		}
	}
	return template.HTML(b.String())
}
