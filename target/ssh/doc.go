// Package ssh defines targets that operate on remote systems via SSH.
//
// SSH targets perform side-effecting operations against a remote host using SSH
// sessions and SFTP for command execution and data transfer. They represent a
// concrete execution context used during planning and execution.
//
// Like all targets, SSH targets are invoked only by the engine and must not
// contain planning, validation, or diagnostic policy logic.
package ssh
