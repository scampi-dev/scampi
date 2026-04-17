// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"filippo.io/age"
	"github.com/charmbracelet/x/term"
	"github.com/urfave/cli/v3"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/diagnostic/event"
	"scampi.dev/scampi/errs"
	"scampi.dev/scampi/osutil"
	"scampi.dev/scampi/secret"
)

const envSecretsFile = "SCAMPI_SECRETS_FILE"

// Diagnostic types
// -----------------------------------------------------------------------------

type secretsInfo struct {
	diagnostic.Info
	Detail string
}

func (e *secretsInfo) Error() string { return e.Detail }

func (e *secretsInfo) EventTemplate() event.Template {
	return event.Template{
		ID:   errs.Code("secrets.Info"),
		Text: "{{.Detail}}",
		Data: e,
	}
}

// Helpers
// -----------------------------------------------------------------------------

func emitSecretsInfo(em diagnostic.Emitter, detail string) {
	em.EmitEngineDiagnostic(diagnostic.RaiseEngineDiagnostic("", &secretsInfo{
		Detail: detail,
	}))
}

func secretsEmitter(ctx context.Context) (diagnostic.Emitter, func()) {
	opts := mustGlobalOpts(ctx)
	displ, cleanup := withDisplayer(opts, nil)
	pol := diagnostic.Policy{Verbosity: opts.verbosity}
	em := diagnostic.NewEmitter(pol, displ)
	return em, cleanup
}

func cliError(msg string) error {
	return cli.Exit(msg, exitUserError)
}

func cliErrorHint(msg, hint string) error {
	return cli.Exit(msg+"\n  hint: "+hint, exitUserError)
}

// resolveSecretsFile returns the secrets file path from (in order):
// --file flag, $SCAMPI_SECRETS_FILE, or auto-discovery in PWD.
func resolveSecretsFile(cmd *cli.Command) string {
	if f := cmd.String("file"); f != "" {
		return f
	}
	if f := os.Getenv(envSecretsFile); f != "" {
		return f
	}
	for _, name := range []string{"secrets.age.json", "secrets.file.json"} {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}
	return ""
}

// resolveSecretsFileOrDefault returns the secrets file path, falling
// back to "secrets.age.json" in PWD when nothing is configured. Used
// by write commands that should create the file on first use.
func resolveSecretsFileOrDefault(cmd *cli.Command) string {
	if f := resolveSecretsFile(cmd); f != "" {
		return f
	}
	return "secrets.age.json"
}

// resolveIdentities returns the age identities, checking the --identity
// flag on the parent secrets command first, then falling back to the
// standard resolution (env vars, default path).
func resolveIdentities(cmd *cli.Command) ([]age.Identity, error) {
	if f := cmd.String("identity"); f != "" {
		data, err := os.ReadFile(f)
		if err != nil {
			// bare-error: CLI boundary, reported via cli.Exit
			return nil, errs.Errorf("reading identity file %s: %w", f, err)
		}
		return age.ParseIdentities(strings.NewReader(string(data)))
	}
	return secret.ResolveIdentities(os.LookupEnv, os.ReadFile)
}

func readStore(filePath string) (map[string]string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var store map[string]string
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	return store, nil
}

func writeStore(filePath string, store map[string]string) error {
	out, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, append(out, '\n'), 0644)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func countRecipients(wrapped string) int {
	if !secret.IsAgeEncrypted(wrapped) {
		return 0
	}
	encoded := wrapped[len("AGE[") : len(wrapped)-len("]")]
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return 0
	}
	return strings.Count(string(raw), "-> X25519 ")
}

func requireSecretsFile(cmd *cli.Command) (string, error) {
	f := resolveSecretsFile(cmd)
	if f == "" {
		return "", cliErrorHint(
			"no secrets file found",
			"use --file or create secrets.age.json",
		)
	}
	return f, nil
}

func requireIdentityAsRecipient(cmd *cli.Command) (*age.X25519Identity, []age.Identity, error) {
	identities, err := resolveIdentities(cmd)
	if err != nil {
		return nil, nil, cliError(err.Error())
	}
	id, ok := identities[0].(*age.X25519Identity)
	if !ok {
		return nil, nil, cliError("identity is not an X25519 key")
	}
	return id, identities, nil
}

func parseRecipients(cmd *cli.Command, self *age.X25519Identity) ([]age.Recipient, error) {
	recipients := []age.Recipient{self.Recipient()}
	for _, s := range cmd.StringSlice("recipient") {
		r, err := age.ParseX25519Recipient(s)
		if err != nil {
			return nil, cliError(fmt.Sprintf("invalid recipient %q: %s", s, err))
		}
		recipients = append(recipients, r)
	}
	return recipients, nil
}

var secretsFileFlag = &cli.StringFlag{
	Name:    "file",
	Aliases: []string{"f"},
	Usage:   "path to secrets file (default: secrets.age.json in PWD)",
}

// Command tree
// -----------------------------------------------------------------------------

func secretsCmd() *cli.Command {
	return &cli.Command{
		Name:  "secrets",
		Usage: "Manage age-encrypted secrets",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "identity",
				Aliases: []string{"i"},
				Usage:   "path to age identity file (default: ~/.config/scampi/age.key)",
			},
		},
		Commands: []*cli.Command{
			secretsInitCmd(),
			secretsPubkeyCmd(),
			secretsSetCmd(),
			secretsGetCmd(),
			secretsListCmd(),
			secretsDelCmd(),
			secretsInfoCmd(),
			secretsRecryptCmd(),
		},
	}
}

// scampi secrets init
// -----------------------------------------------------------------------------

func secretsInitCmd() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Generate an age keypair for encrypting secrets",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "force",
				Usage: "overwrite existing key file",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			em, cleanup := secretsEmitter(ctx)
			defer cleanup()

			dir, err := osutil.UserConfigDir()
			if err != nil {
				return cliError("cannot determine config directory: " + err.Error())
			}
			keyPath := secret.DefaultAgeKeyPath(dir)
			force := cmd.Bool("force")

			if _, err := os.Stat(keyPath); err == nil {
				if !force {
					return cliErrorHint(
						"key file already exists: "+keyPath,
						"use --force to overwrite",
					)
				}
				if !term.IsTerminal(os.Stdin.Fd()) {
					return cliError(
						"refusing to overwrite key in non-interactive mode — this is destructive and irreversible",
					)
				}
				_, _ = fmt.Fprintf(os.Stderr,
					"WARNING: this will permanently destroy %s\n"+
						"Any secrets encrypted with this key will become unrecoverable.\n"+
						"Type YES to confirm: ", keyPath)
				var confirm string
				_, _ = fmt.Scanln(&confirm)
				if confirm != "YES" {
					return cliError("aborted")
				}
			}

			identity, err := age.GenerateX25519Identity()
			if err != nil {
				return cliError("generating keypair: " + err.Error())
			}

			keyDir := filepath.Dir(keyPath)
			if err := os.MkdirAll(keyDir, 0700); err != nil {
				return cliError("creating directory " + keyDir + ": " + err.Error())
			}

			if err := os.WriteFile(keyPath, []byte(identity.String()+"\n"), 0600); err != nil {
				return cliError("writing key file " + keyPath + ": " + err.Error())
			}

			emitSecretsInfo(em, "identity saved to "+keyPath)
			_, _ = fmt.Println(identity.Recipient().String())
			return nil
		},
	}
}

// scampi secrets pubkey
// -----------------------------------------------------------------------------

func secretsPubkeyCmd() *cli.Command {
	return &cli.Command{
		Name:  "pubkey",
		Usage: "Print the public key for the current age identity",
		Action: func(_ context.Context, cmd *cli.Command) error {
			identities, err := resolveIdentities(cmd)
			if err != nil {
				return cliError(err.Error())
			}

			id, ok := identities[0].(*age.X25519Identity)
			if !ok {
				return cliError("identity is not an X25519 key")
			}

			_, _ = fmt.Println(id.Recipient().String())
			return nil
		},
	}
}

// scampi secrets set
// -----------------------------------------------------------------------------

func secretsSetCmd() *cli.Command {
	return &cli.Command{
		Name:      "set",
		Usage:     "Encrypt and store a secret value",
		ArgsUsage: "<key> [value]",
		Flags: []cli.Flag{
			secretsFileFlag,
			&cli.StringSliceFlag{
				Name:    "recipient",
				Aliases: []string{"r"},
				Usage:   "additional age recipient public key (repeatable)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			em, cleanup := secretsEmitter(ctx)
			defer cleanup()

			nargs := cmd.Args().Len()
			if nargs < 1 || nargs > 2 {
				return usageError(cmd, "expected <key> [value]")
			}

			key := cmd.Args().First()
			filePath := resolveSecretsFileOrDefault(cmd)

			id, _, err := requireIdentityAsRecipient(cmd)
			if err != nil {
				return err
			}
			recipients, err := parseRecipients(cmd, id)
			if err != nil {
				return err
			}

			var plaintext string
			if nargs == 2 {
				plaintext = cmd.Args().Get(1)
			} else {
				if term.IsTerminal(os.Stdin.Fd()) {
					return usageError(cmd, "no value: pass as second argument or pipe to stdin")
				}
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return cliError("reading stdin: " + err.Error())
				}
				plaintext = string(data)
			}

			if plaintext == "" {
				return usageError(cmd, "refusing to store empty secret — pass a value as the second argument")
			}

			encrypted, err := secret.EncryptValue(plaintext, recipients)
			if err != nil {
				return cliError("encrypting value: " + err.Error())
			}

			store := make(map[string]string)
			if data, err := os.ReadFile(filePath); err == nil && len(data) > 0 {
				if err := json.Unmarshal(data, &store); err != nil {
					return cliError(fmt.Sprintf("parsing %s: %s", filePath, err))
				}
			}

			store[key] = encrypted

			if err := writeStore(filePath, store); err != nil {
				return cliError(fmt.Sprintf("writing %s: %s", filePath, err))
			}

			emitSecretsInfo(em, fmt.Sprintf("encrypted %q into %s", key, filePath))
			return nil
		},
	}
}

// scampi secrets list
// -----------------------------------------------------------------------------

func secretsListCmd() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List available secret keys",
		Flags: []cli.Flag{secretsFileFlag},
		Action: func(_ context.Context, cmd *cli.Command) error {
			filePath, err := requireSecretsFile(cmd)
			if err != nil {
				return err
			}

			store, err := readStore(filePath)
			if err != nil {
				return cliError(fmt.Sprintf("reading %s: %s", filePath, err))
			}

			for _, k := range sortedKeys(store) {
				_, _ = fmt.Println(k)
			}
			return nil
		},
	}
}

// scampi secrets get
// -----------------------------------------------------------------------------

func secretsGetCmd() *cli.Command {
	return &cli.Command{
		Name:      "get",
		Usage:     "Decrypt and print a secret value (or list all keys if no key given)",
		ArgsUsage: "[key]",
		Flags:     []cli.Flag{secretsFileFlag},
		Before:    requireMaxArgs(1),
		Action: func(_ context.Context, cmd *cli.Command) error {
			filePath, err := requireSecretsFile(cmd)
			if err != nil {
				return err
			}

			store, err := readStore(filePath)
			if err != nil {
				return cliError(fmt.Sprintf("reading %s: %s", filePath, err))
			}

			if cmd.Args().Len() == 0 {
				for _, k := range sortedKeys(store) {
					_, _ = fmt.Println(k)
				}
				return nil
			}

			key := cmd.Args().First()
			identities, err := resolveIdentities(cmd)
			if err != nil {
				return cliError(err.Error())
			}

			encrypted, ok := store[key]
			if !ok {
				return cliErrorHint(
					fmt.Sprintf("key %q not found in %s", key, filePath),
					"run secrets list to see available keys",
				)
			}

			plaintext, err := secret.DecryptValue(encrypted, identities)
			if err != nil {
				return cliError(fmt.Sprintf("decrypting %q: %s", key, err))
			}

			_, _ = fmt.Print(plaintext)
			return nil
		},
	}
}

// scampi secrets del
// -----------------------------------------------------------------------------

func secretsDelCmd() *cli.Command {
	return &cli.Command{
		Name:      "del",
		Usage:     "Remove a secret from the store",
		ArgsUsage: "<key>",
		Flags:     []cli.Flag{secretsFileFlag},
		Before:    requireArgs(1),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			em, cleanup := secretsEmitter(ctx)
			defer cleanup()

			filePath, err := requireSecretsFile(cmd)
			if err != nil {
				return err
			}

			key := cmd.Args().First()
			store, err := readStore(filePath)
			if err != nil {
				return cliError(fmt.Sprintf("reading %s: %s", filePath, err))
			}

			if _, ok := store[key]; !ok {
				return cliError(fmt.Sprintf("key %q not found in %s", key, filePath))
			}

			delete(store, key)

			if err := writeStore(filePath, store); err != nil {
				return cliError(fmt.Sprintf("writing %s: %s", filePath, err))
			}

			emitSecretsInfo(em, fmt.Sprintf("deleted %q from %s", key, filePath))
			return nil
		},
	}
}

// scampi secrets info
// -----------------------------------------------------------------------------

func secretsInfoCmd() *cli.Command {
	return &cli.Command{
		Name:  "info",
		Usage: "Show secret keys and their recipient counts",
		Flags: []cli.Flag{secretsFileFlag},
		Action: func(_ context.Context, cmd *cli.Command) error {
			filePath, err := requireSecretsFile(cmd)
			if err != nil {
				return err
			}

			store, err := readStore(filePath)
			if err != nil {
				return cliError(fmt.Sprintf("reading %s: %s", filePath, err))
			}

			for _, k := range sortedKeys(store) {
				n := countRecipients(store[k])
				_, _ = fmt.Fprintf(os.Stdout, "%s  %d recipient(s)\n", k, n)
			}
			return nil
		},
	}
}

// scampi secrets recrypt
// -----------------------------------------------------------------------------

func secretsRecryptCmd() *cli.Command {
	return &cli.Command{
		Name:  "recrypt",
		Usage: "Re-encrypt all secrets with the current identity and specified recipients",
		Flags: []cli.Flag{
			secretsFileFlag,
			&cli.StringSliceFlag{
				Name:    "recipient",
				Aliases: []string{"r"},
				Usage:   "age recipient public key (repeatable; replaces existing recipients)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			em, cleanup := secretsEmitter(ctx)
			defer cleanup()

			filePath, err := requireSecretsFile(cmd)
			if err != nil {
				return err
			}

			id, identities, err := requireIdentityAsRecipient(cmd)
			if err != nil {
				return err
			}
			recipients, err := parseRecipients(cmd, id)
			if err != nil {
				return err
			}

			if len(recipients) == 1 {
				if !term.IsTerminal(os.Stdin.Fd()) {
					return cliError(
						"no --recipient given — this would drop all other recipients; " +
							"pass -r explicitly or run interactively",
					)
				}
				_, _ = fmt.Fprintf(os.Stderr,
					"No --recipient flags given. This will re-encrypt all secrets\n"+
						"for only your key (%s), dropping all other recipients.\n"+
						"Type YES to confirm: ",
					id.Recipient().String(),
				)
				var confirm string
				_, _ = fmt.Scanln(&confirm)
				if confirm != "YES" {
					return cliError("aborted")
				}
			}

			store, err := readStore(filePath)
			if err != nil {
				return cliError(fmt.Sprintf("reading %s: %s", filePath, err))
			}

			for k, v := range store {
				plain, err := secret.DecryptValue(v, identities)
				if err != nil {
					return cliError(fmt.Sprintf("decrypting %q: %s", k, err))
				}
				encrypted, err := secret.EncryptValue(plain, recipients)
				if err != nil {
					return cliError(fmt.Sprintf("re-encrypting %q: %s", k, err))
				}
				store[k] = encrypted
			}

			if err := writeStore(filePath, store); err != nil {
				return cliError(fmt.Sprintf("writing %s: %s", filePath, err))
			}

			emitSecretsInfo(em, fmt.Sprintf(
				"re-encrypted %d key(s) in %s with %d recipient(s)",
				len(store),
				filePath,
				len(recipients),
			))
			return nil
		},
	}
}

func usageError(cmd *cli.Command, msg string) error {
	_, _ = fmt.Fprintf(os.Stderr, "Incorrect Usage: %s\n\n", msg)
	_ = cli.ShowSubcommandHelp(cmd)
	return cli.Exit("", exitUserError)
}
