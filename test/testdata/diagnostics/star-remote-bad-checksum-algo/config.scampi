target.local(name="host")
deploy(name="main", targets=["host"], steps=[
    copy(src=remote(url="https://example.com/file.tar.gz", checksum="crc32:deadbeef"), dest="/tmp/out", perm="0644", owner="root", group="root"),
])
