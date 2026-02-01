package test

import "godoit.dev/doit/builtin"

target: builtin.local

steps: [
	builtin.symlink & {
		desc:   "link in missing directory"
		target: "/target.txt"
		link:   "/nonexistent/link.txt"
	},
]
