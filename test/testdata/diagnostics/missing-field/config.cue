package sandbox

import "godoit.dev/doit/builtin"

units: [
	builtin.copy & {
		name:  "builtin.copy action"
		src:   "./.src1.yml"
		dest:  "./.dest1.yml"
		owner: "user"
		group: "group"
	},
	{
		name:  "anon copy action"
		src:   "./.src1.yml"
		dest:  "./.dest1.yml"
		perm:  "0644"
		owner: "user"
		group: "group"
	},
]
