target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        pkg(
            desc="upgrade packages",
            packages=["nginx"],
            state="latest",
            source=system(),
        ),
    ],
)
