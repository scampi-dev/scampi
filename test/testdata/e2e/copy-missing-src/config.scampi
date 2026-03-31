target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        copy(
            dest="/etc/app/config.txt",
            perm="0644",
            owner="root",
            group="root",
        ),
    ],
)
