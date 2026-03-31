target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        service(
            desc="restart nginx",
            name="nginx",
            state="restarted",
        ),
    ],
)
