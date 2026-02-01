package test

import "godoit.dev/doit/builtin"

target: builtin.local

steps: [
	builtin.symlink & {
		desc:   "link path is a regular file"
		target: "/target.txt"
		link:   "/link.txt"
	},
]
