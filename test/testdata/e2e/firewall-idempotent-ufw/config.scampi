target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        firewall(
            desc="SSH already allowed",
            port="22/tcp",
            action="allow",
        ),
    ],
)
