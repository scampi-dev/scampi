target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        run(
            desc="ip forwarding already enabled",
            check="check-ip-forward",
            apply="apply-ip-forward",
        ),
    ],
)
