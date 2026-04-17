// SPDX-License-Identifier: GPL-3.0-only

package ssh

import "scampi.dev/scampi/errs"

const (
	CodeNoKnownHosts   errs.Code = "ssh.NoKnownHosts"
	CodeNoSuchHost     errs.Code = "ssh.NoSuchHost"
	CodeConnection     errs.Code = "ssh.Connection"
	CodeUnknownKey     errs.Code = "ssh.UnknownKey"
	CodeKeyMismatch    errs.Code = "ssh.KeyMismatch"
	CodeKeyRevoked     errs.Code = "ssh.KeyRevoked"
	CodeKeyRead        errs.Code = "ssh.KeyRead"
	CodeKeyParse       errs.Code = "ssh.KeyParse"
	CodeNoAuthMethod   errs.Code = "ssh.NoAuthMethod"
	CodeAuth           errs.Code = "ssh.Auth"
	CodeInvalidTimeout errs.Code = "ssh.InvalidTimeout"
	CodeSFTPSession    errs.Code = "ssh.SFTPSession"
)
