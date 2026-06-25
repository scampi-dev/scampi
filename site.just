# Constants
# -----------------------------------------------------------------------------

build_dir       := f"{{justfile_directory()}}/build"
out_dir         := f"{{build_dir}}/site"
dist_dir        := f"{{build_dir}}/site-dist"
infra_dir       := env("SCAMPI_INFRA", f"{{justfile_directory()}}/../infra")
hugo            := "go -C site tool github.com/gohugoio/hugo"
mdlint          := "go -C site tool github.com/shinagawa-web/gomarklint"

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
  {{hugo}} --minify --destination {{out_dir}}

[group('build')]
[doc("Local dev server")]
dev *args: _copy-static
  {{hugo}} server --disableFastRender -D -e production --destination {{out_dir}} {{args}}

[group('build')]
[doc("Package site for deployment")]
dist: clean build
  mkdir -p {{dist_dir}}
  tar czf {{dist_dir}}/site.tar.gz -C {{out_dir}} .

[private]
_copy-static:
  cp scampi.png site/static/scampi.png
  cp scampi-sec.png site/static/scampi-sec.png
  cp scripts/install.sh site/static/install.sh

# Deploy
# -----------------------------------------------------------------------------

[group('deploy')]
[doc("Stage site tarball to scampi-infra")]
stage: dist
  cp {{dist_dir}}/site.tar.gz {{infra_dir}}/files/site.tar.gz

[group('deploy')]
[doc("Build and deploy site via scampi-infra")]
deploy: stage
  cd {{infra_dir}} && scampi apply site.scampi

# Quality
# -----------------------------------------------------------------------------

[group('quality')]
[doc("Lint markdown content")]
lint:
  {{mdlint}} content/

# Cleanup
# -----------------------------------------------------------------------------

[group('cleanup')]
[doc("Remove all generated files")]
clean:
  rm -rf {{out_dir}} {{dist_dir}} site/resources/_gen
  rm -f site/static/scampi.png
  rm -f site/static/scampi-sec.png
  rm -f site/static/install.sh
