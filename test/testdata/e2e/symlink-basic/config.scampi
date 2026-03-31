target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        symlink(
            desc="create symlink",
            target="/tmp/target.txt",
            link="/tmp/link.txt",
        ),
    ],
)
