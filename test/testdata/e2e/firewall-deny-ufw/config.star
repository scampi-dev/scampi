target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        firewall(
            desc="deny HTTPS",
            port="443/tcp",
            action="deny",
        ),
    ],
)
