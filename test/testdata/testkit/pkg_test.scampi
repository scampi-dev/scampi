t = test.target.in_memory(name = "mock")

deploy(
    name = "packages",
    targets = ["mock"],
    steps = [
        pkg(packages = ["nginx", "curl"], source = system()),
    ],
)

a = test.assert.that(t)

a.package("nginx").is_installed()
a.package("curl").is_installed()
a.package("apache2").is_absent()
