target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        symlink(
            desc="link path is a regular file",
            target="/tmp/target.txt",
            link="/tmp/link.txt",
        ),
    ],
)
