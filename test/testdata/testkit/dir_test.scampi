t = test.target.in_memory(name = "mock")

deploy(
    name = "dirs",
    targets = ["mock"],
    steps = [
        dir(path = "/var/www"),
        dir(path = "/var/www/mysite"),
    ],
)

assert_that = test.assert.that(t)

assert_that.dir("/var/www").exists()
assert_that.dir("/var/www/mysite").exists()
assert_that.dir("/nonexistent").absent()
