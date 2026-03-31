target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="render greeting",
            src=local("/tmpl/greeting.txt"),
            dest="/tmp/greeting.txt",
            data={
                "values": {
                    "name": "world",
                    "count": 3,
                },
            },
            perm="0644",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
