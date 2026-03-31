target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        group(
            desc="remove group appusers",
            name="appusers",
            state="absent",
        ),
    ],
)
