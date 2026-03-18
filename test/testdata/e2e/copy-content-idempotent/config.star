target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        copy(
            desc="already-present sudoers entry",
            src=inline("hal9000 ALL=(ALL) NOPASSWD:ALL\n"),
            dest="/etc/sudoers.d/hal9000",
            perm="0440",
            owner="testuser",
            group="testgroup",
        ),
    ],
)
