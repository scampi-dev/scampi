target.local(name="host")
deploy(name="main", targets=["host"], steps=[
    copy(src=remote(url="://missing-scheme"), dest="/tmp/out", perm="0644", owner="root", group="root"),
])
