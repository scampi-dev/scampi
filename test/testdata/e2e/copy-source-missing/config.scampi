target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        copy(
            desc="copy missing file",
            src=local("/tmp/nonexistent.txt"),
            dest="/tmp/out.txt",
            perm="0644",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
