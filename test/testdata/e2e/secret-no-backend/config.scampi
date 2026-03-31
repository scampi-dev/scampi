target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="no backend configured",
            content="pass={{.pw}}",
            dest="/tmp/out.txt",
            data={
                "values": {
                    "pw": secret("db_pass"),
                },
            },
            perm="0644",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
