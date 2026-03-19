target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        pkg(
            desc="packages already installed",
            packages=["nginx", "curl"],
            source=system(),
        ),
    ],
)
