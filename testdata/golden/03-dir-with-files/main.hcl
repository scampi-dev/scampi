dir "nested" {
  path = "{{TMP}}/nested"
}

file "readme" {
  path    = "{{TMP}}/nested/README.md"
  content = "scampi reconciled this directory\n"
}

file "config" {
  path    = "{{TMP}}/nested/app.conf"
  content = <<-EOT
    name = scampi
    mode = real
  EOT
}
