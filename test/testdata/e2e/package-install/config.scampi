target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        pkg(
            desc="install packages",
            packages=["nginx", "curl"],
            source=system(),
        ),
    ],
)
