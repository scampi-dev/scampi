target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        copy(
            desc="copy with bad permission",
            src=local("/src.txt"),
            dest="/dest.txt",
            perm="invalid",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
