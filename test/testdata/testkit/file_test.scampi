t = test.target.in_memory(
    name = "mock",
    files = {"/etc/existing.conf": "old content"},
)

deploy(
    name = "files",
    targets = ["mock"],
    steps = [
        copy(
            desc = "write config",
            dest = "/etc/app.conf",
            src = inline("server_name example.com\nlisten 80\n"),
            perm = "0644",
            owner = "root",
            group = "root",
        ),
    ],
)

a = test.assert.that(t)

a.file("/etc/app.conf").exists()
a.file("/etc/app.conf").contains("server_name example.com")
a.file("/etc/app.conf").has_content("server_name example.com\nlisten 80\n")
a.file("/etc/existing.conf").exists()
a.file("/nonexistent").absent()
