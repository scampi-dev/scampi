package symlink

#Step: {
	@doc("Create and manage symbolic links")
	@example("""
		builtin.symlink & {
		    target: "/opt/app/config.yaml"
		    link:   "/etc/app/config.yaml"
		}
		""")

	close({
		_kind:  "symlink"
		desc?:  string @doc("Human-readable description")
		target: string @doc("Path the symlink points to (like ln -s TARGET)")
		link:   string @doc("Path where symlink is created (like ln -s ... LINK)")
	})
}
