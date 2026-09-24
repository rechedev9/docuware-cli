---
title: Configuration
description: "Where dw keeps profiles, secrets, tokens and cached metadata, and every environment variable it reads."
---

# Configuration

`dw` has no configuration file to edit by hand: `dw login` writes everything. This page says where it goes.

## Files

| What | Windows | macOS | Linux |
|---|---|---|---|
| Profiles (`config.json`) | `%APPDATA%\dw\` | `~/Library/Application Support/dw/` | `$XDG_CONFIG_HOME/dw/` or `~/.config/dw/` |
| Secrets fallback (`secrets.json`) | same folder, only without a keyring | same | same |
| Tokens and metadata cache | `%LOCALAPPDATA%\dw\` | `~/Library/Caches/dw/` | `$XDG_CACHE_HOME/dw/` or `~/.cache/dw/` |
| Claude Code skill | `%USERPROFILE%\.claude\skills\docuware\` | `~/.claude/skills/docuware/` | `~/.claude/skills/docuware/` |

`DW_CONFIG_DIR` and `DW_CACHE_DIR` override the first and third rows. Files are written atomically with owner-only
permissions (`0600`, folders `0700`).

`config.json` looks like this (no secrets):

```json
{
  "current": "default",
  "profiles": {
    "default": {"url": "https://acme.docuware.cloud", "method": "password", "username": "peggy.jenkins"}
  }
}
```

The cache folder has one subfolder per server + account (a hash), holding `token.json` and `meta/`.

## Metadata cache

File cabinets, dialogs and dialog fields change rarely and DocuWare Cloud rate-limits some of those endpoints, so dw
caches them for one hour per account. After an administrator changes a dialog, use `--no-cache` (or `DW_NO_CACHE=1`)
once, or wait an hour. `dw logout` and `dw login` clear the cache of that account.

## Environment variables

| Variable | Effect |
|---|---|
| `DW_URL` | server; overrides the profile's |
| `DW_USERNAME`, `DW_PASSWORD` | password-grant credentials; override the profile's |
| `DW_CLIENT_ID`, `DW_CLIENT_SECRET` | client-credentials grant; win over `DW_USERNAME` |
| `DW_PROFILE` | profile to use when `-p` is not given |
| `DW_INSECURE` | `1`/`true`: skip TLS verification |
| `DW_NO_CACHE` | `1`/`true`: ignore the metadata cache |
| `DW_SECRET_STORE` | `file`: store secrets in `secrets.json` instead of the OS keyring |
| `DW_CONFIG_DIR` | folder for `config.json` and `secrets.json` |
| `DW_CACHE_DIR` | folder for tokens and metadata |
| `HTTPS_PROXY`, `HTTP_PROXY`, `NO_PROXY` | standard proxy settings |

## Network behaviour

- Requests time out when the server sends no response headers for 90 seconds.
- HTTP `429` (DocuWare Cloud's rate limit, about 60 calls per minute on some endpoints) is retried up to 3 times,
  waiting as long as `Retry-After` says, capped at 30 seconds per wait.
- HTTP `401` triggers one token renewal and retry.
