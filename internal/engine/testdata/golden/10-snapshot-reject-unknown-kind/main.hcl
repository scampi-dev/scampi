file "would-write" {
  path    = "{{TMP}}/should-not-exist.txt"
  content = "snapshot is bad so this never lands"
}

frob "bad" {
  attr = "value"
}
