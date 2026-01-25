package test

import "godoit.dev/doit/nonexistent"

steps: [
	nonexistent.step & {
		desc: "broken"
	},
]
