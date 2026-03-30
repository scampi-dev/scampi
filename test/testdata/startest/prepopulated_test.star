# Test that pre-populated target state is preserved and modified correctly.
# The target starts with existing files, packages, and services — steps
# should converge without clobbering unrelated state.

t = test.target.in_memory(
    name = "mock",
    files = {
        "/etc/hostname": "old-host",
        "/etc/motd": "welcome",
    },
    packages = ["curl", "git"],
    services = {"sshd": "running"},
    dirs = ["/home/deploy"],
)

deploy(
    name = "converge",
    targets = ["mock"],
    steps = [
        # Overwrite hostname
        copy(
            desc = "set hostname",
            dest = "/etc/hostname",
            src = inline("new-host"),
            perm = "0644",
            owner = "root",
            group = "root",
        ),
        # Install additional package
        pkg(packages = ["nginx"], source = system()),
        # Add a directory
        dir(path = "/var/log/app"),
    ],
)

a = test.assert.that(t)

# Changed state
a.file("/etc/hostname").has_content("new-host")
a.package("nginx").is_installed()
a.dir("/var/log/app").exists()

# Preserved state — not touched by steps
a.file("/etc/motd").has_content("welcome")
a.package("curl").is_installed()
a.package("git").is_installed()
a.service("sshd").is_running()
a.dir("/home/deploy").exists()
