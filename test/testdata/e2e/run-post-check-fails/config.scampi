target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        run(
            desc="apply succeeds but check still fails",
            check="check-thing",
            apply="apply-thing",
        ),
    ],
)
