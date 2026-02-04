package test

import "godoit.dev/doit/builtin"

target: builtin.local

steps: [
	builtin.copy & {
		desc:  "copy hello.txt"
		src:   "/tmp/src.txt"
		dest:  "/tmp/dest.txt"
		perm:  "0755"
		owner: "testuser"
		group: "testgroup"
	},
]
