target.local(name="host")
deploy(name="main", targets=["host"], steps=[
    run(apply="echo hi", check="true", always=True),
])
