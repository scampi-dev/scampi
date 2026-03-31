target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="render inline",
            src=inline("server={{ .host }}:{{ .port }}"),
            dest="/tmp/config.txt",
            data={
                "values": {
                    "host": "localhost",
                    "port": 8080,
                },
            },
            perm="0600",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
