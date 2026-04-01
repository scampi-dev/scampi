t = test.target.in_memory(name = "mock")

deploy(
    name = "links",
    targets = ["mock"],
    steps = [
        dir(path = "/opt/app"),
        dir(path = "/opt/app/releases/v1"),
        symlink(
            link = "/opt/app/current",
            target = "/opt/app/releases/v1",
        ),
        copy(
            desc = "app config",
            dest = "/opt/app/releases/v1/config.yml",
            src = inline("port: 8080\nenv: production\n"),
            perm = "0644",
            owner = "root",
            group = "root",
        ),
    ],
)

a = test.assert.that(t)

a.symlink("/opt/app/current").points_to("/opt/app/releases/v1")
a.file("/opt/app/releases/v1/config.yml").contains("port: 8080")
a.file("/opt/app/releases/v1/config.yml").contains("env: production")
a.dir("/opt/app/releases/v1").exists()
a.symlink("/opt/app/nonexistent").absent()
