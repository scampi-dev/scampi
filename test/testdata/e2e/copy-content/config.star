target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        copy(
            desc="passwordless sudo for automation user",
            src=inline("hal9000 ALL=(ALL) NOPASSWD:ALL\n"),
            dest="/etc/sudoers.d/hal9000",
            perm="0440",
            owner="root",
            group="root",
        ),
    ],
)
