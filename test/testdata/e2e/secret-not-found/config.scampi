secrets(backend="file", path="/secrets.json")

target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="missing secret",
            content="token={{.token}}",
            dest="/tmp/out.txt",
            data={
                "values": {
                    "token": secret("nonexistent_key"),
                },
            },
            perm="0644",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
