target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        group(
            desc="group already exists",
            name="appusers",
        ),
    ],
)
