target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        user(
            desc="create system user",
            name="appd",
            system=True,
            shell="/usr/sbin/nologin",
        ),
    ],
)
