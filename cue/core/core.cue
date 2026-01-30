package core

import "godoit.dev/doit/builtin"

unit?: close({
	id!:   string
	desc?: string
})

target: builtin.#BuiltinTarget

steps: [...builtin.#BuiltinStep]
