dir "a" {
  path = "${dir.b.path}/a"
}

dir "b" {
  path = "${dir.a.path}/b"
}
