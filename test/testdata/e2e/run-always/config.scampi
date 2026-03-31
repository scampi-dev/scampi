target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        run(
            desc="refresh something",
            apply="do-the-thing",
            always=True,
        ),
    ],
)
