target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        run(
            desc="always-run that fails",
            apply="fail-cmd",
            always=True,
        ),
    ],
)
