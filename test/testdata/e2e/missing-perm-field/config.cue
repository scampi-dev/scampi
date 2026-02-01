package test

import "godoit.dev/doit/builtin"

target: builtin.local

steps: [
	builtin.copy & {
		desc:  "copy without perm"
		src:   "/src.txt"
		dest:  "/dest.txt"
		owner: "testuser"
		group: "testgroup"
	},
]
