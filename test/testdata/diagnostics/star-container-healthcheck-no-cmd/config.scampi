target.local(name="host")
deploy(name="main", targets=["host"], steps=[
    container.instance(
        name="test",
        image="nginx:latest",
        healthcheck=container.healthcheck.cmd(interval="10s"),
    ),
])
