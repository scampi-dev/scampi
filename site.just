hugo := "go tool github.com/gohugoio/hugo"
mdlint := "go tool github.com/shinagawa-web/gomarklint"

[default]
[doc("Build the Hugo site")]
build:
  cd site && {{hugo}} --minify

[doc("Local dev server")]
dev:
  cd site && {{hugo}} server -D -e production

[doc("Package site for deployment")]
dist outdir="build/site-dist": build
  mkdir -p {{outdir}}
  tar czf {{outdir}}/site.tar.gz -C build/site .

[doc("Lint markdown content")]
lint:
  cd site && {{mdlint}} content/

[doc("Remove all generated files")]
clean:
  rm -rf build/site build/site-dist site/resources/_gen
