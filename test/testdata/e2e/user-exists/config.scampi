target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        user(
            desc="user already exists",
            name="hal9000",
            shell="/bin/bash",
            home="/home/hal9000",
        ),
    ],
)
