-- Project-local DAP setup for doit
local dap = require("dap")
dap.configurations.go = dap.configurations.go or {}

-- Run Config - APPLY
local apply_config = {
	type = "go",
	name = "doit: apply sandbox",
	request = "launch",
	program = "${workspaceFolder}/cmd",
	args = {
		"apply",
		"./sandbox/config.cue",
	},
	cwd = "${workspaceFolder}",
}

-- Register them (also makes them available to :DapContinue if you want)
table.insert(dap.configurations.go, apply_config)

-- Keymaps
local function dap_run(cfg)
	return function()
		dap.run(cfg)
	end
end

vim.keymap.set("n", "<leader>dd", function() end, {
	desc = "doit debug",
})

vim.keymap.set("n", "<leader>dda", dap_run(apply_config), {
	desc = "doit: debug apply",
})

return {}
