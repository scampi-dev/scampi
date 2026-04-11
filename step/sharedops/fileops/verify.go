// SPDX-License-Identifier: GPL-3.0-only

package fileops

import (
	"context"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"path"
	"strings"

	"scampi.dev/scampi/target"
)

// VerifiedWrite writes content to a temp file, runs verifyCmd against it,
// and only writes to dest if the command exits 0. The temp file is always
// cleaned up. The verifyCmd must contain exactly one %s which is replaced
// with the temp file path.
//
// The temp file preserves the basename of dest so that tools which
// auto-detect format from the filename (e.g. Caddy, nginx) work correctly.
func VerifiedWrite(
	ctx context.Context,
	tgt target.Target,
	dest string,
	content []byte,
	verifyCmd string,
) error {
	// Reject anything other than exactly one `%s` up front. Literal
	// verify strings are caught at link time by the @std.pattern
	// attribute on copy.verify / template.verify, so this guards
	// the runtime path where the value came from std.env, std.secret,
	// or another non-literal source.
	if n := strings.Count(verifyCmd, "%s"); n != 1 {
		return &VerifyPlaceholderError{Cmd: verifyCmd, Count: n}
	}

	fsTgt := target.Must[target.Filesystem]("verify", tgt)
	cmdTgt := target.Must[target.Command]("verify", tgt)

	tmpDir := tempDir()
	tmpFile := path.Join(tmpDir, path.Base(dest))

	if err := fsTgt.Mkdir(ctx, tmpDir, fs.FileMode(0o700)); err != nil {
		return newVerifyIOError("create temp dir", err)
	}
	defer func() {
		_ = fsTgt.Remove(ctx, tmpFile)
		_ = fsTgt.Remove(ctx, tmpDir)
	}()

	if err := fsTgt.WriteFile(ctx, tmpFile, content); err != nil {
		return newVerifyIOError("write temp file", err)
	}

	cmd := strings.Replace(verifyCmd, "%s", tmpFile, 1)
	result, err := cmdTgt.RunCommand(ctx, cmd)
	if err != nil {
		return newVerifyIOError("run verify command", err)
	}
	if result.ExitCode != 0 {
		return &VerifyError{
			Cmd:      verifyCmd,
			Dest:     dest,
			ExitCode: result.ExitCode,
			Stderr:   result.Stderr,
		}
	}

	return fsTgt.WriteFile(ctx, dest, content)
}

func tempDir() string {
	return fmt.Sprintf("/tmp/.scampi-%016x", rand.Uint64())
}
