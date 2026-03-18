target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        copy(
            desc="copy to nonexistent dir",
            src=local("/tmp/src.txt"),
            dest="/tmp/subdir/dest.txt",
            perm="0644",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
