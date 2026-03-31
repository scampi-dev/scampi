target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        dir(
            desc="path is a regular file",
            path="/tmp/mydir",
        ),
    ],
)
