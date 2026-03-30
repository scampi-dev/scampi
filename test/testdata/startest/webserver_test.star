t = test.target.in_memory(
    name = "mock",
    packages = ["nginx"],
    services = {"nginx": "stopped"},
)

deploy(
    name = "webserver",
    targets = ["mock"],
    steps = [
        dir(path = "/var/www"),
        dir(path = "/var/www/mysite"),
        dir(path = "/etc/nginx/sites-enabled"),
        copy(
            desc = "nginx site config",
            dest = "/etc/nginx/sites-enabled/mysite.conf",
            src = inline("server {\n    server_name example.com;\n    root /var/www/mysite;\n}\n"),
            perm = "0644",
            owner = "root",
            group = "root",
        ),
        service(name = "nginx", state = "running"),
    ],
)

a = test.assert.that(t)

# Directories created
a.dir("/var/www").exists()
a.dir("/var/www/mysite").exists()

# Config file written with correct content
a.file("/etc/nginx/sites-enabled/mysite.conf").exists()
a.file("/etc/nginx/sites-enabled/mysite.conf").contains("server_name example.com")
a.file("/etc/nginx/sites-enabled/mysite.conf").contains("root /var/www/mysite")

# Service started
a.service("nginx").is_running()
a.service("nginx").is_enabled()

# Nothing unexpected
a.file("/etc/nginx/sites-enabled/default").absent()
a.dir("/var/www/other").absent()
