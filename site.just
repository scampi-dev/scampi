hugo      := "go tool github.com/gohugoio/hugo"
mdlint    := "go tool github.com/shinagawa-web/gomarklint"
infra_dir := env("SCAMPI_INFRA", "../scampi-infra")

export HUGO_SCAMPI_VERSION := env("HUGO_SCAMPI_VERSION", `git describe --tags --match 'v*' --abbrev=0 2>/dev/null || echo ""`)
export HUGO_SCAMPI_GO_VERSION := `grep '^go ' go.mod | awk '{print $2}'`

# Help
# -----------------------------------------------------------------------------

[default]
[doc("Show this help message")]
@help *args:
  just --unsorted --list site {{args}}

# Build
# -----------------------------------------------------------------------------

[group('build')]
[doc("Build the Hugo site")]
build: _copy-static
  cd site && {{hugo}} --minify

[group('build')]
[doc("Local dev server")]
dev *args: _copy-static
  cd site && {{hugo}} server --disableFastRender -D -e production {{args}}

[group('build')]
[doc("Package site for deployment")]
dist outdir="build/site-dist": build
  mkdir -p {{outdir}}
  tar czf {{outdir}}/site.tar.gz -C build/site .

[private]
_copy-static:
  cp scampi.png site/static/scampi.png
  cp scampi-sec.png site/static/scampi-sec.png
  cp scripts/install.sh site/static/install.sh

# Deploy
# -----------------------------------------------------------------------------

[group('deploy')]
[doc("Build and deploy site via scampi-infra")]
deploy: dist
  cp build/site-dist/site.tar.gz {{infra_dir}}/files/site.tar.gz
  cd {{infra_dir}} && scampi apply site.scampi

# Quality
# -----------------------------------------------------------------------------

[group('quality')]
[doc("Lint markdown content")]
lint:
  cd site && {{mdlint}} content/

# Cleanup
# -----------------------------------------------------------------------------

[group('cleanup')]
[doc("Remove all generated files")]
clean:
  rm -rf build/site build/site-dist site/resources/_gen
  rm site/static/scampi.png
  rm site/static/scampi-sec.png
  rm site/static/install.sh
