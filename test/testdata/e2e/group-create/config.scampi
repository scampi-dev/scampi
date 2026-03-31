target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        group(
            desc="create group appusers",
            name="appusers",
            gid=1100,
        ),
    ],
)
