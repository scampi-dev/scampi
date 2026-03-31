target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="missing source",
            src=local("/does/not/exist.tmpl"),
            dest="/tmp/out.txt",
            perm="0644",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
