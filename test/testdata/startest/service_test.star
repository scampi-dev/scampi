t = test.target.in_memory(
    name = "mock",
    services = {"nginx": "stopped"},
)

deploy(
    name = "services",
    targets = ["mock"],
    steps = [
        service(name = "nginx", state = "running"),
    ],
)

a = test.assert.that(t)

a.service("nginx").is_running()
a.service("nginx").is_enabled()
