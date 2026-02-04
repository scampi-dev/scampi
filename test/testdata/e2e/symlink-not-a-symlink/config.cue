package test

import "godoit.dev/doit/builtin"

target: builtin.local

steps: [
	builtin.symlink & {
		desc:   "link path is a regular file"
		target: "/tmp/target.txt"
		link:   "/tmp/link.txt"
	},
]
