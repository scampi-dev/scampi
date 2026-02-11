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
				desc:     "packages already installed"
				packages: ["nginx", "curl"]
			},
		]
	}
}
