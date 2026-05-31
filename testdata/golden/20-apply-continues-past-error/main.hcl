file "bad" {
  path    = "{{TMP}}/missing-parent/bad.txt"
  content = "this fails because the parent does not exist"
}

file "good" {
  path    = "{{TMP}}/good.txt"
  content = "yes\n"
}
