target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        user(
            desc="remove user hal9000",
            name="hal9000",
            state="absent",
        ),
    ],
)
