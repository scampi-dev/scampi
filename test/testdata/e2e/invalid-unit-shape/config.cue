package test

import "godoit.dev/doit/builtin"

// unit must be a struct with id field, not a string
unit: "invalid"

steps: [
	builtin.copy & {
		desc:  "copy file"
		src:   "/src.txt"
		dest:  "/dest.txt"
		perm:  "0644"
		owner: "testuser"
		group: "testgroup"
	},
]
