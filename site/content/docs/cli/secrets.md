---
title: secrets
weight: 7
---

Manage [age](https://age-encryption.org/)-encrypted secrets. All subcommands
operate on a JSON secrets file, specified either by the `--file` flag or the
`SCAMPI_SECRETS_FILE` environment variable.

## How age encryption works

Every developer runs `scampi secrets init` to generate their own age keypair,
stored in `$XDG_CONFIG_HOME/scampi/` (typically `~/.config/scampi/`). The
private key stays on their machine; the public key is shared with the team.

When you `set` a secret, it's encrypted to your key plus any additional
recipients you specify with `--recipient`. Anyone whose public key was included
as a recipient can decrypt that secret with their private key. Each secret in
the JSON file is encrypted independently, so different secrets can have
different recipient lists.

To grant a new team member access, re-set the secrets they need with their
public key added as a `--recipient`.

## Key rotation and revocation

Age bakes the recipient list into the ciphertext. You can't add or remove
recipients without re-encrypting.

**Lost key**: If a team member loses their private key, they lose the ability to
decrypt. Other recipients are unaffected — anyone with a valid key can still
`get` the secrets and re-set them without the lost key as a recipient.

**Revoking access**: To revoke a recipient (e.g. someone leaving the team),
`get` each secret they had access to, then `set` it again with only the
recipients you want to keep. The old ciphertext is replaced, and the revoked
key can no longer decrypt the new value.

In both cases the workflow is the same: decrypt with a working key, re-encrypt
to the updated recipient list. Always include at least two recipients so a
single lost key doesn't make secrets unrecoverable.

## init

```text
scampi secrets init [--force]
```

Generate an age keypair for encrypting secrets.

| Flag      | Description                 |
| --------- | --------------------------- |
| `--force` | Overwrite existing key file |

## pubkey

```text
scampi secrets pubkey
```

Print the public key for the current age identity.

## set

```text
scampi secrets set [flags] <key> [value]
```

Encrypt and store a secret value. If `value` is omitted, it is read from stdin.

| Flag                | Description                                                     |
| ------------------- | --------------------------------------------------------------- |
| `-f`, `--file`      | Path to the secrets JSON file (default: `$SCAMPI_SECRETS_FILE`) |
| `-r`, `--recipient` | Additional age recipient public key (repeatable)                |

## get

```text
scampi secrets get [flags] <key>
```

Decrypt and print a secret value.

| Flag           | Description                                                     |
| -------------- | --------------------------------------------------------------- |
| `-f`, `--file` | Path to the secrets JSON file (default: `$SCAMPI_SECRETS_FILE`) |

## del

```text
scampi secrets del [flags] <key>
```

Remove a secret from the store.

| Flag           | Description                                                     |
| -------------- | --------------------------------------------------------------- |
| `-f`, `--file` | Path to the secrets JSON file (default: `$SCAMPI_SECRETS_FILE`) |
