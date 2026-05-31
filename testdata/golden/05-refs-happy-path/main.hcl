dir "etc" {
  path = "{{TMP}}/etc"
}

file "motd" {
  path    = "${dir.etc.path}/motd"
  content = "welcome to ${dir.etc.path}\n"
}
