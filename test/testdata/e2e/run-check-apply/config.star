target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        run(
            desc="enable ip forwarding",
            check="check-ip-forward",
            apply="apply-ip-forward",
        ),
    ],
)
