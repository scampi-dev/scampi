target.local(name="local")

deploy(
    name="test",
    targets=["local"],
    steps=[
        pkg(
            desc="already configured dnf repo",
            packages=["example-pkg"],
            source=dnf_repo(
                url="https://example.com/repo/el9",
            ),
        ),
    ],
)
