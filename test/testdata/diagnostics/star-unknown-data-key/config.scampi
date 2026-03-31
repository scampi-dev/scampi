target.local(name="host")
deploy(name="main", targets=["host"], steps=[
    template(
        dest="/tmp/out",
        perm="0644",
        owner="root",
        group="root",
        src=inline("hello"),
        data={"bad_key": "val"},
    ),
])
