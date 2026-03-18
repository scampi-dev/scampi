target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="bad syntax",
            src=inline("hello {{.name"),
            dest="/tmp/out.txt",
            perm="0644",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
