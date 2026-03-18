target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        copy(
            desc="copy hello.txt",
            src=local("/tmp/src.txt"),
            dest="/tmp/dest.txt",
            perm="0755",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
