target.local(name="host")
deploy(name="main", targets=["host"], steps=[
    container.instance(name="test", image="nginx:latest", mounts=["yolo"]),
])
