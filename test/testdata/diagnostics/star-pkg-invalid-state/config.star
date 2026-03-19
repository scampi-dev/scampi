target.local(name="host")
deploy(name="main", targets=["host"], steps=[
    pkg(packages=["vim"], state="purge", source=system()),
])
