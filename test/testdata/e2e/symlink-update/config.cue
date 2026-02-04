package test

import "godoit.dev/doit/builtin"

target: builtin.local

steps: [
	builtin.symlink & {
		desc:   "update symlink target"
		target: "/tmp/new-target.txt"
		link:   "/tmp/link.txt"
	},
]
