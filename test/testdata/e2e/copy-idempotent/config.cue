package test

import "godoit.dev/doit/builtin"

target: builtin.local

steps: [
	builtin.copy & {
		desc:  "copy already-present file"
		src:   "/tmp/src.txt"
		dest:  "/tmp/dest.txt"
		perm:  "0644"
		owner: "testuser"
		group: "testgroup"
	},
]
