package test

import "godoit.dev/doit/builtin"

target: builtin.local

steps: [
	builtin.symlink & {
		desc:   "symlink already correct"
		target: "/tmp/target.txt"
		link:   "/tmp/link.txt"
	},
]
