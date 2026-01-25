package test

import "godoit.dev/doit/builtin"

// Reference to undefined variable
steps: [
	builtin.copy & {
		desc:  "copy file"
		src:   undefinedVar
		dest:  "/dest.txt"
		perm:  "0644"
		owner: "testuser"
		group: "testgroup"
	},
]
