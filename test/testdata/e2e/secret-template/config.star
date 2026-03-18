secrets(backend="file", path="/secrets.json")

target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="render with secret",
            src=inline("dsn=postgres://app:{{.db_pass}}@db:5432/myapp"),
            dest="/tmp/dsn.txt",
            data={
                "values": {
                    "db_pass": secret("db_pass"),
                },
            },
            perm="0600",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
