package ssh

#Target: {
	@doc("Remote POSIX system via SSH")
	@example("""
        builtin.ssh & {
            host: "prod.example.com"
            user: "deploy"
        }
        """)

	close({
		kind:     "ssh"
		host:     string        @doc("Hostname or IP address")
		port:     *22 | int     @doc("SSH port (default: 22)")
		user:     string        @doc("SSH username")
		key?:     string        @doc("Path to private key (default: SSH agent)")
		insecure: *false | bool @doc("Skip host key verification (default: false)")
	})
}
