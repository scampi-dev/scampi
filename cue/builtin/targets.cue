package builtin

import (
	tlocal "godoit.dev/doit/targets/local"
	tssh "godoit.dev/doit/targets/ssh"
)

#BuiltinTarget: tlocal.#Target | tssh.#Target
local:          tlocal.#Target
ssh:            tssh.#Target
