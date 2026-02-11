package pkg

#Step: {
	@doc("Ensure packages are present or absent on the target")
	@example("""
		builtin.pkg & {
		    packages: ["nginx", "curl", "git"]
		}
		""")
	@example("""
		builtin.pkg & {
		    packages: ["telnetd"]
		    state:    "absent"
		}
		""")

	close({
		kind:     "pkg"
		desc?:    string @doc("Human-readable description")
		packages: [...string] & [_, ...] @doc("Packages to ensure present or absent")
		state:    *"present" | "absent" @doc("Desired package state")
	})
}
