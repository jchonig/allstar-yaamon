# Passkeys (WebAuthn / FIDO2)

YAAMon supports FIDO2 passkey authentication using the WebAuthn standard. Users can sign in with a platform authenticator (Touch ID, Face ID, Windows Hello), a hardware security key (YubiKey, etc.), or a FIDO2-compatible password manager (Bitwarden, 1Password, iCloud Keychain).

## Zero-config operation

YAAMon derives the WebAuthn Relying Party ID and origin automatically from the `Origin` (or `Host`) header of each request. Passkeys work out of the box with any hostname — no configuration is required for most deployments.

## Explicit configuration

Explicit configuration is only needed when automatic derivation would produce the wrong hostname (e.g., when accessed behind a reverse proxy that rewrites the `Origin` header):

```yaml
webauthn:
  rpid: "yourdomain.example"          # Relying Party ID — must be a registered domain suffix
  rp_origins:
    - "https://yourdomain.example"    # Allowed WebAuthn origins
```

| Key | Default | Description |
|-----|---------|-------------|
| `webauthn.rpid` | derived from `Origin`/`Host` header | Relying Party ID (domain, no port) |
| `webauthn.rp_origins` | derived from `Origin`/`Host` header | Allowed WebAuthn origins |

When both `webauthn.rpid` and `webauthn.rp_origins` are set, those values take precedence for all ceremonies.

## How to use passkeys

See the [User Guide — Profile](../user-guide/profile.md#passkeys) for instructions on registering passkeys, signing in, renaming, and deleting credentials.
