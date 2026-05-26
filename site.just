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
dev *args: _copy-static _stage-bench
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
  ./scripts/gen-install-scripts.sh

# Stage CI bench baselines into site/static/raw-bench/ so `bench.md`'s
# JS can fetch them at the same /raw-bench/<suffix>/... paths it uses
# in production (where Caddy serves /var/www/scampi.dev/raw-bench/
# directly, outside the Hugo build). Local-only — dist/deploy don't
# use this dir.
[private]
_stage-bench:
  #!/usr/bin/env bash
  set -euo pipefail
  rm -rf site/static/raw-bench
  shopt -s nullglob
  for f in benchmarks/*-ci-gh-*.txt; do
    name="$(basename "$f")"
    suffix="$(echo "$name" | sed -E 's/^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{4}-//; s/\.txt$//')"
    dst="site/static/raw-bench/$suffix"
    mkdir -p "$dst"
    cp "$f" "$dst/"
  done
  for dir in site/static/raw-bench/*/; do
    (cd "$dir" && ls -1 *.txt | jq -R . | jq -s . > index.json)
  done

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
  rm -rf site/static/raw-bench
  rm site/static/scampi.png
  rm site/static/scampi-sec.png
  rm site/static/cli.sh
  rm site/static/install.sh
  rm site/static/lsp.sh
