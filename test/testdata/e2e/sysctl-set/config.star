target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        sysctl(
            desc="enable IP forwarding",
            key="net.ipv4.ip_forward",
            value="1",
        ),
    ],
)
