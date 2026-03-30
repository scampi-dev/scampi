# Test multiple deploy blocks against the same target.
# Each block converges a different part of the system.

t = test.target.in_memory(name = "mock")

deploy(
    name = "base",
    targets = ["mock"],
    steps = [
        dir(path = "/etc/app"),
        dir(path = "/var/log/app"),
        pkg(packages = ["curl"], source = system()),
    ],
)

deploy(
    name = "app",
    targets = ["mock"],
    steps = [
        copy(
            desc = "app config",
            dest = "/etc/app/config.yml",
            src = inline("debug: false\nport: 3000\n"),
            perm = "0644",
            owner = "root",
            group = "root",
        ),
    ],
)

a = test.assert.that(t)

# From base deploy
a.dir("/etc/app").exists()
a.dir("/var/log/app").exists()
a.package("curl").is_installed()

# From app deploy
a.file("/etc/app/config.yml").exists()
a.file("/etc/app/config.yml").contains("debug: false")
a.file("/etc/app/config.yml").contains("port: 3000")
