package test

import "godoit.dev/doit/nonexistent"

target: builtin.local

steps: [
	nonexistent.step & {
		desc: "broken"
	},
]
