// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"filippo.io/age"
	"github.com/charmbracelet/x/term"
	"github.com/urfave/cli/v3"

	"scampi.dev/scampi/osutil"
	"scampi.dev/scampi/secret"
)

const envSecretsFile = "SCAMPI_SECRETS_FILE"

func usageError(cmd *cli.Command, msg string) error {
	_, _ = fmt.Fprintf(os.Stderr, "Incorrect Usage: %s\n\n", msg)
	_ = cli.ShowSubcommandHelp(cmd)
	return cli.Exit("", exitUserError)
}

func resolveSecretsFile(cmd *cli.Command) string {
	if f := cmd.String("file"); f != "" {
		return f
	}
	return os.Getenv(envSecretsFile)
}

func secretsCmd() *cli.Command {
	return &cli.Command{
		Name:  "secrets",
		Usage: "Manage age-encrypted secrets",
		Commands: []*cli.Command{
			secretsInitCmd(),
			secretsPubkeyCmd(),
			secretsSetCmd(),
			secretsGetCmd(),
			secretsDelCmd(),
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
		Action: func(_ context.Context, cmd *cli.Command) error {
			dir, err := osutil.UserConfigDir()
			if err != nil {
				return cli.Exit(fmt.Sprintf("cannot determine config directory: %s", err), exitUserError)
			}
			keyPath := secret.DefaultAgeKeyPath(dir)
			force := cmd.Bool("force")

			if !force {
				if _, err := os.Stat(keyPath); err == nil {
					return cli.Exit(
						fmt.Sprintf("key file already exists: %s (use --force to overwrite)", keyPath),
						exitUserError,
					)
				}
			}

			identity, err := age.GenerateX25519Identity()
			if err != nil {
				return cli.Exit(fmt.Sprintf("generating keypair: %s", err), exitUserError)
			}

			keyDir := filepath.Dir(keyPath)
			if err := os.MkdirAll(keyDir, 0700); err != nil {
				return cli.Exit(fmt.Sprintf("creating directory %s: %s", keyDir, err), exitUserError)
			}

			if err := os.WriteFile(keyPath, []byte(identity.String()+"\n"), 0600); err != nil {
				return cli.Exit(fmt.Sprintf("writing key file %s: %s", keyPath, err), exitUserError)
			}

			_, _ = fmt.Fprintf(os.Stderr, "identity saved to %s\n", keyPath)
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
		Action: func(_ context.Context, _ *cli.Command) error {
			identities, err := secret.ResolveIdentities(os.LookupEnv, os.ReadFile)
			if err != nil {
				return cli.Exit(err.Error(), exitUserError)
			}

			id, ok := identities[0].(*age.X25519Identity)
			if !ok {
				return cli.Exit("identity is not an X25519 key", exitUserError)
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
			&cli.StringFlag{
				Name:    "file",
				Aliases: []string{"f"},
				Usage:   "path to the secrets JSON file (default: $SCAMPI_SECRETS_FILE)",
			},
			&cli.StringSliceFlag{
				Name:    "recipient",
				Aliases: []string{"r"},
				Usage:   "additional age recipient public key (repeatable)",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			nargs := cmd.Args().Len()
			if nargs < 1 || nargs > 2 {
				return usageError(cmd, "expected <key> [value]")
			}

			key := cmd.Args().First()
			filePath := resolveSecretsFile(cmd)
			if filePath == "" {
				return usageError(cmd, "no secrets file: use --file or set $SCAMPI_SECRETS_FILE")
			}

			// Derive own public key from identity as default recipient
			identities, err := secret.ResolveIdentities(os.LookupEnv, os.ReadFile)
			if err != nil {
				return cli.Exit(err.Error(), exitUserError)
			}
			id, ok := identities[0].(*age.X25519Identity)
			if !ok {
				return cli.Exit("identity is not an X25519 key", exitUserError)
			}

			recipients := []age.Recipient{id.Recipient()}
			for _, s := range cmd.StringSlice("recipient") {
				r, err := age.ParseX25519Recipient(s)
				if err != nil {
					return cli.Exit(fmt.Sprintf("invalid recipient %q: %s", s, err), exitUserError)
				}
				recipients = append(recipients, r)
			}

			// Value from second arg, or stdin (but not an interactive terminal)
			var plaintext string
			if nargs == 2 {
				plaintext = cmd.Args().Get(1)
			} else {
				if term.IsTerminal(os.Stdin.Fd()) {
					return usageError(cmd, "no value: pass as second argument or pipe to stdin")
				}
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return cli.Exit(fmt.Sprintf("reading stdin: %s", err), exitUserError)
				}
				plaintext = string(data)
			}

			encrypted, err := secret.EncryptValue(plaintext, recipients)
			if err != nil {
				return cli.Exit(fmt.Sprintf("encrypting value: %s", err), exitUserError)
			}

			store := make(map[string]string)
			if data, err := os.ReadFile(filePath); err == nil {
				if err := json.Unmarshal(data, &store); err != nil {
					return cli.Exit(fmt.Sprintf("parsing %s: %s", filePath, err), exitUserError)
				}
			}

			store[key] = encrypted

			out, err := json.MarshalIndent(store, "", "  ")
			if err != nil {
				return cli.Exit(fmt.Sprintf("marshaling JSON: %s", err), exitUserError)
			}

			if err := os.WriteFile(filePath, append(out, '\n'), 0644); err != nil {
				return cli.Exit(fmt.Sprintf("writing %s: %s", filePath, err), exitUserError)
			}

			_, _ = fmt.Fprintf(os.Stderr, "encrypted %q into %s\n", key, filePath)
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
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "file",
				Aliases: []string{"f"},
				Usage:   "path to the secrets JSON file (default: $SCAMPI_SECRETS_FILE)",
			},
		},
		Before: requireArgs(1),
		Action: func(_ context.Context, cmd *cli.Command) error {
			key := cmd.Args().First()
			filePath := resolveSecretsFile(cmd)
			if filePath == "" {
				return usageError(cmd, "no secrets file: use --file or set $SCAMPI_SECRETS_FILE")
			}

			data, err := os.ReadFile(filePath)
			if err != nil {
				return cli.Exit(fmt.Sprintf("reading %s: %s", filePath, err), exitUserError)
			}

			var store map[string]string
			if err := json.Unmarshal(data, &store); err != nil {
				return cli.Exit(fmt.Sprintf("parsing %s: %s", filePath, err), exitUserError)
			}

			if _, ok := store[key]; !ok {
				return cli.Exit(fmt.Sprintf("key %q not found in %s", key, filePath), exitUserError)
			}

			delete(store, key)

			out, err := json.MarshalIndent(store, "", "  ")
			if err != nil {
				return cli.Exit(fmt.Sprintf("marshaling JSON: %s", err), exitUserError)
			}

			if err := os.WriteFile(filePath, append(out, '\n'), 0644); err != nil {
				return cli.Exit(fmt.Sprintf("writing %s: %s", filePath, err), exitUserError)
			}

			_, _ = fmt.Fprintf(os.Stderr, "deleted %q from %s\n", key, filePath)
			return nil
		},
	}
}

// scampi secrets get
// -----------------------------------------------------------------------------

func secretsGetCmd() *cli.Command {
	return &cli.Command{
		Name:      "get",
		Usage:     "Decrypt and print a secret value",
		ArgsUsage: "<key>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "file",
				Aliases: []string{"f"},
				Usage:   "path to the secrets JSON file (default: $SCAMPI_SECRETS_FILE)",
			},
		},
		Before: requireArgs(1),
		Action: func(_ context.Context, cmd *cli.Command) error {
			key := cmd.Args().First()
			filePath := resolveSecretsFile(cmd)
			if filePath == "" {
				return usageError(cmd, "no secrets file: use --file or set $SCAMPI_SECRETS_FILE")
			}

			identities, err := secret.ResolveIdentities(os.LookupEnv, os.ReadFile)
			if err != nil {
				return cli.Exit(err.Error(), exitUserError)
			}

			data, err := os.ReadFile(filePath)
			if err != nil {
				return cli.Exit(fmt.Sprintf("reading %s: %s", filePath, err), exitUserError)
			}

			var store map[string]string
			if err := json.Unmarshal(data, &store); err != nil {
				return cli.Exit(fmt.Sprintf("parsing %s: %s", filePath, err), exitUserError)
			}

			encrypted, ok := store[key]
			if !ok {
				return cli.Exit(fmt.Sprintf("key %q not found in %s", key, filePath), exitUserError)
			}

			plaintext, err := secret.DecryptValue(encrypted, identities)
			if err != nil {
				return cli.Exit(fmt.Sprintf("decrypting %q: %s", key, err), exitUserError)
			}

			_, _ = fmt.Print(plaintext)
			return nil
		},
	}
}
