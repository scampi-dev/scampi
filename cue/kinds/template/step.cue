package template

#BaseStep: {
	@doc("Render templates with data substitution and owner/permission management")
	@example("""
		builtin.template & {
		    src:  "./nginx.conf.tmpl"
		    dest: "/etc/nginx/nginx.conf"
		    data: {
		        values: {
		            port: 8080
		            workers: 4
		        }
		        env: {
		            PORT: "port"
		        }
		    }
		    perm:  "0644"
		    owner: "root"
		    group: "root"
		}
		""")

	kind:  "template"
	desc?: string @doc("Human-readable description")
	dest:  string @doc("Output file path")
	data?: {
		values?: {
			[string]: _ @doc("Inline data values (defines schema and defaults)")
		}
		env?: {
			[string]: string @doc("Environment variable mappings (ENV_VAR: \"key\")")
		}
	} @doc("Data sources for template rendering")
	perm:  string @doc("File permissions in octal (e.g. \"0644\")")
	owner: string @doc("Owner user name or UID")
	group: string @doc("Group name or GID")
}

#TemplateSource: {
	matchN(==1, [
		{src!: string},
		{content!: string},
	])
}

#Step: {
	#BaseStep
	#TemplateSource
}
