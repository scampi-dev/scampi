target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        pkg(
            desc="install from dnf repo",
            packages=["example-pkg"],
            source=dnf_repo(
                url="https://example.com/repo/el9",
            ),
        ),
    ],
)
