---
title: Authentication
description: "How dw signs in to DocuWare: OAuth2 password and client-credentials grants, profiles, the OS keyring, token renewal and environment variables."
---

# Authentication

DocuWare's Platform API uses OAuth2 (required for API integrations since DocuWare 7.11). `dw` supports two ways in:

| Method | Login | Acts as | Use for |
|---|---|---|---|
| DocuWare user | `dw login --url <server> --user <name>` | that user, with that user's rights | people, agents on a desktop |
| OAuth app (client credentials) | `dw login --url <server> --client-id <id>` | the app registration | unattended jobs, if the server allows it |

**Single sign-on accounts cannot use the API.** DocuWare does not support SSO (Microsoft/Entra ID, ADFS, ...) for API
access. Use a DocuWare user that has a DocuWare password, or an OAuth app. See [DocuWare setup](docuware-setup.md).

## What `dw login` does

1. Normalizes `--url`: `acme` becomes `https://acme.docuware.cloud`; `dms.example.com` becomes
   `https://dms.example.com`; a full URL keeps its scheme and host (any path is dropped). Use the scheme for host names
   without dots (`https://dwserver`).
2. Reads the secret: from stdin with `--password-stdin`, else from `DW_PASSWORD` / `DW_CLIENT_SECRET`, else from a
   hidden prompt. Without a terminal and without the other two it stops with exit code `2`.
3. Discovers the token endpoint: `GET /DocuWare/Platform/Home/IdentityServiceInfo` → the Identity Service URL →
   `/.well-known/openid-configuration` → `token_endpoint`.
4. Requests a token (password grant with client `docuware.platform.net.client`, scope `docuware.platform`; or
   client credentials with your id and secret) and reads the server version to prove it works.
5. Saves the profile, stores the secret in the OS keyring, and caches the token.

Nothing is saved when the sign-in fails.

## Profiles

A profile is a saved server + account. `dw login` saves to the profile named by `-p` (default `default`) and makes it
current. Pick another one per call with `-p <name>` or per shell with `DW_PROFILE=<name>`.

```sh
dw login --url acme --user peggy.jenkins                 # profile "default"
dw login --url acme-test --user peggy.jenkins -p test    # profile "test", now current
dw -p default status
```

`dw logout` (with `-p` for another profile) removes the profile, its secret and its cached tokens and metadata.

## Where secrets go

- The password or client secret is stored in the OS keyring under the service `dw-cli`: Windows Credential Manager,
  macOS Keychain, or the Secret Service on Linux (GNOME Keyring, KWallet).
- If no keyring is reachable (headless Linux, containers) or `DW_SECRET_STORE=file` is set, it goes to
  `secrets.json` in the config folder with owner-only permissions, and `dw login` prints a warning.
- `config.json` never holds secrets. File locations: [Configuration](configuration.md#files).

## Tokens

- Access tokens last 60 minutes by default. They are cached per server and account and reused across runs.
- When a token expires or DocuWare answers `401`, dw requests a new one with the stored secret and retries once.
- The token endpoint is cached too, so later runs skip discovery; a `404` from it triggers discovery again.
- Tokens are sent only to the configured host, even if a response links to another host.

## Environment variables

For CI jobs, containers and one-off runs, credentials can come from the environment. They override the profile.

| Variable | Meaning |
|---|---|
| `DW_URL` | server (same forms as `--url`); required when there is no profile |
| `DW_USERNAME` + `DW_PASSWORD` | password grant |
| `DW_CLIENT_ID` + `DW_CLIENT_SECRET` | client credentials grant (wins when both pairs are set) |
| `DW_INSECURE=1` | skip TLS checks |
| `DW_PROFILE` | profile to use when no `-p` is given |

```sh
DW_URL=acme DW_USERNAME=svc-reader DW_PASSWORD="$SECRET" dw search Invoices STATUS=Open --json
```

Tokens obtained this way are cached like profile tokens. Do not use environment credentials to work around the
password prompt on a desktop: a secret in an environment variable ends up in shell history and process listings.

## On-premises servers

- `--url https://dms.example.com` (or `http://` if the server really is plain HTTP).
- Self-signed certificate: prefer adding the CA to the OS trust store. `--insecure` (or `DW_INSECURE=1`) skips
  verification for that profile; use it only on a network you trust.
- Proxies: the standard `HTTPS_PROXY`, `HTTP_PROXY` and `NO_PROXY` variables are honored.
- DocuWare 7.10 or newer is needed for OAuth2; 7.11+ is what DocuWare requires for API integrations.
