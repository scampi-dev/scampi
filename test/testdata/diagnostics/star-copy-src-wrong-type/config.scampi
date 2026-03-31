target.local(name="host")
deploy(name="main", targets=["host"], steps=[
    copy(src="./file.txt", dest="/tmp/out", perm="0644", owner="root", group="root"),
])
