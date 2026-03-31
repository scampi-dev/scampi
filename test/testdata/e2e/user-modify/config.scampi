target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        user(
            desc="modify user shell and groups",
            name="hal9000",
            shell="/bin/zsh",
            home="/home/hal9000",
            groups=["sudo", "docker"],
        ),
    ],
)
