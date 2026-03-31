target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        dir(
            desc="create directory with permissions",
            path="/tmp/mydir",
            perm="0700",
        ),
    ],
)
