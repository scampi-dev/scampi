hugo      := "go tool github.com/gohugoio/hugo"
mdlint    := "go tool github.com/shinagawa-web/gomarklint"
infra_dir := env("SCAMPI_INFRA", "../scampi-infra")

export HUGO_SCAMPI_VERSION := env("HUGO_SCAMPI_VERSION", `git describe --tags --match 'v*' --abbrev=0 2>/dev/null || echo ""`)
export HUGO_SCAMPI_GO_VERSION := `grep '^go ' go.mod | awk '{print $2}'`

[default]
[doc("Build the Hugo site")]
build:
  cp scampi.png site/static/scampi.png
  cp scripts/install.sh site/static/install.sh
  cd site && {{hugo}} --minify

[doc("Local dev server")]
dev *args:
  cp scampi.png site/static/scampi.png
  cp scripts/install.sh site/static/install.sh
  cd site && {{hugo}} server -D -e production {{args}}

[doc("Package site for deployment")]
dist outdir="build/site-dist": build
  mkdir -p {{outdir}}
  tar czf {{outdir}}/site.tar.gz -C build/site .

[doc("Lint markdown content")]
lint:
  cd site && {{mdlint}} content/

[doc("Build and deploy site via scampi-infra")]
deploy: dist
  cp build/site-dist/site.tar.gz {{infra_dir}}/files/site.tar.gz
  cd {{infra_dir}} && scampi apply site.scampi

[doc("Remove all generated files")]
clean:
  rm -rf build/site build/site-dist site/resources/_gen
