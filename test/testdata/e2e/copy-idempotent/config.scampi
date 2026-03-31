target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        copy(
            desc="copy already-present file",
            src=local("/tmp/src.txt"),
            dest="/tmp/dest.txt",
            perm="0644",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
