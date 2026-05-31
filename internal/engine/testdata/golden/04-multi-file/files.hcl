file "motd" {
  path    = "{{TMP}}/multi/etc/motd"
  content = "welcome\n"
}

file "hosts" {
  path    = "{{TMP}}/multi/etc/hosts"
  content = "127.0.0.1 localhost\n"
}
