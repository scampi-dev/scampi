package copy

#Step: {
	@doc("Copy files with owner and permission management")
	@example("""
		builtin.copy & {
		    src:   "./config.yaml"
		    dest:  "/etc/app/config.yaml"
		    perm:  "0644"
		    owner: "root"
		    group: "root"
		}
		""")

	close({
		_kind: "copy"
		desc?: string @doc("Human-readable description")
		src:   string @doc("Source file path")
		dest:  string @doc("Destination file path")
		perm:  string @doc("File permissions in octal (e.g. \"0644\")")
		owner: string @doc("Owner user name or UID")
		group: string @doc("Group name or GID")
	})
}
