target.local(name="host")
deploy(name="main", targets=["host"], steps=[
    copy(src=remote(url="ftp://files.example.com/config.tar.gz"), dest="/tmp/out", perm="0644", owner="root", group="root"),
])
