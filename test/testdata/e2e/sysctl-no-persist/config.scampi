target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        sysctl(
            desc="set live only",
            key="net.ipv4.tcp_keepalive_time",
            value="300",
            persist=False,
        ),
    ],
)
