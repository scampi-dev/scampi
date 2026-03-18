target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        copy(
            desc="copy with invalid perm",
            src=local("/tmp/src.txt"),
            dest="/tmp/dest.txt",
            perm="bad",
            owner="root",
            group="root",
        ),
    ],
)
