target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        run(
            desc="apply that fails",
            check="check-thing",
            apply="apply-thing",
        ),
    ],
)
