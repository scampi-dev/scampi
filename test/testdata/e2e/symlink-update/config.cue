package test

import "godoit.dev/doit/builtin"

target: builtin.local

steps: [
	builtin.symlink & {
		desc:   "update symlink target"
		target: "/new-target.txt"
		link:   "/link.txt"
	},
]
