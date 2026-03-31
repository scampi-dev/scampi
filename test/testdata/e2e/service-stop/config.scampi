target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        service(
            desc="stop nginx",
            name="nginx",
            state="stopped",
            enabled=False,
        ),
    ],
)
