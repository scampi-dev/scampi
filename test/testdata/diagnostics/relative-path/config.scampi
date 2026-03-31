target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        copy(
            desc="copy with relative dest",
            src=local("/tmp/src.txt"),
            dest="./relative/dest.txt",
            perm="0644",
            owner="user",
            group="group",
        ),
    ],
)
