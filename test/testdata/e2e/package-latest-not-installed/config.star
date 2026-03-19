target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        pkg(
            desc="install missing package at latest",
            packages=["nginx"],
            state="latest",
            source=system(),
        ),
    ],
)
