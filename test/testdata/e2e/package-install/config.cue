package test

import "godoit.dev/doit/builtin"

targets: {
	local: builtin.local
}

deploy: {
	test: {
		targets: ["local"]
		steps: [
			builtin.pkg & {
				desc:     "install packages"
				packages: ["nginx", "curl"]
			},
		]
	}
}
