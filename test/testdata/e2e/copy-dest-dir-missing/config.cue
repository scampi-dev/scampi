package test

import "godoit.dev/doit/builtin"

target: builtin.local

steps: [
	builtin.copy & {
		desc:  "copy to nonexistent dir"
		src:   "/src.txt"
		dest:  "/subdir/dest.txt"
		perm:  "0644"
		owner: "testuser"
		group: "testgroup"
	},
]
