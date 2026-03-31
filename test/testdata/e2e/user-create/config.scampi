target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        user(
            desc="create user hal9000",
            name="hal9000",
            shell="/bin/bash",
            home="/home/hal9000",
            groups=["sudo", "docker"],
        ),
    ],
)
