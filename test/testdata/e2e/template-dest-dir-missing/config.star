target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="missing parent dir",
            src=inline("hello"),
            dest="/no/such/dir/out.txt",
            perm="0644",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
