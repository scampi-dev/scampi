package test

import "godoit.dev/doit/builtin"

steps: [
	builtin.copy & {
		desc:  "copy without perm"
		src:   "/src.txt"
		dest:  "/dest.txt"
		owner: "testuser"
		group: "testgroup"
	},
]
