target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        firewall(
            desc="allow SSH",
            port="22/tcp",
            action="allow",
        ),
    ],
)
