target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        symlink(
            desc="link in missing directory",
            target="/tmp/target.txt",
            link="/tmp/nonexistent/link.txt",
        ),
    ],
)
