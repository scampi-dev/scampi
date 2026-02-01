package test

import "godoit.dev/doit/builtin"

target: builtin.local

steps: [
	builtin.copy & {
		desc:  "copy with bad permission"
		src:   "/src.txt"
		dest:  "/dest.txt"
		perm:  "invalid"
		owner: "testuser"
		group: "testgroup"
	},
]
