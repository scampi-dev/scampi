target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        pkg(
            desc="remove packages",
            packages=["tree"],
            state="absent",
        ),
    ],
)
