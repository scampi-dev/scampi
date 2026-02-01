package test

import "godoit.dev/doit/builtin"

target: builtin.local

steps: [
	builtin.symlink & {
		desc:   "create symlink"
		target: "/target.txt"
		link:   "/link.txt"
	},
]
