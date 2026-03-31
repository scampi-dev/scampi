target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        template(
            desc="already rendered",
            src=inline("hello"),
            dest="/tmp/out.txt",
            perm="0644",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
