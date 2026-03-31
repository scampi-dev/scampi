target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        symlink(
            desc="symlink already correct",
            target="/tmp/target.txt",
            link="/tmp/link.txt",
        ),
    ],
)
