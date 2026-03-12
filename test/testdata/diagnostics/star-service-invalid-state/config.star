target.local(name="host")
deploy(name="main", targets=["host"], steps=[
    service(name="nginx", state="bogus"),
])
