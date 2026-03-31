target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        service(
            desc="start nginx",
            name="nginx",
        ),
    ],
)
