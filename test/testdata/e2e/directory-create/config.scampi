target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        dir(
            desc="create new directory",
            path="/tmp/mydir",
        ),
    ],
)
