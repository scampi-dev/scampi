package builtin

import (
	kcopy "godoit.dev/doit/kinds/copy"
	kpkg "godoit.dev/doit/kinds/pkg"
	ksymlink "godoit.dev/doit/kinds/symlink"
	ktemplate "godoit.dev/doit/kinds/template"
)

#BuiltinStep: kcopy.#Step | kpkg.#Step | ksymlink.#Step | ktemplate.#Step
copy:         kcopy.#Step
pkg:          kpkg.#Step
symlink:      ksymlink.#Step
template:     ktemplate.#Step
