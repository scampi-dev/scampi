target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        dir(
            desc="directory already exists",
            path="/tmp/mydir",
        ),
    ],
)
