package test

import "godoit.dev/doit/builtin"

target: builtin.local

// src must be a string, not a number
steps: [
	builtin.copy & {
		desc:  "copy file"
		src:   123
		dest:  "/dest.txt"
		perm:  "0644"
		owner: "testuser"
		group: "testgroup"
	},
]
