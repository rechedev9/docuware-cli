---
title: Troubleshooting
description: "Symptoms, causes and fixes for installing, signing in, searching and using dw from Claude Code."
---

# Troubleshooting

Start with `dw status`. Its exit code and `hint:` line narrow most problems down. Exit codes are listed in
[Output](output.md#exit-codes).

## Install

**`dw: command not found` / `dw is not recognized`**
The folder with `dw` is not on the `PATH` of this shell. A `PATH` change only reaches shells started afterwards: open a
new terminal or restart Claude Code. Meanwhile call it by full path (`%LOCALAPPDATA%\Programs\dw\dw.exe`,
`~/.local/bin/dw`, or `$(go env GOPATH)/bin/dw`).

**`go install` fails with `go.mod requires go >= 1.26`**
Install a newer Go (`winget install --id GoLang.Go -e`, `brew install go`, <https://go.dev/dl/>) or use a release
binary.

**macOS: "dw cannot be opened because the developer cannot be verified"**
The release binaries are not notarized. A binary downloaded with `curl` is not quarantined; one downloaded with a
browser is. Clear the flag with `xattr -d com.apple.quarantine ~/.local/bin/dw`.

## Sign in

**`error: no terminal to prompt for the password; use --password-stdin or set DW_PASSWORD`**
`dw login` ran somewhere without a terminal: through an agent, Claude Code's `!` prefix, or a pipe. Run it in a normal
terminal window.

**`error: not logged in to DocuWare` (exit 3)**
There is no profile, or the command used another one. `dw login` again, or pass the right `-p <profile>`. Check
whether `DW_PROFILE` is set.

**`error: DocuWare rejected the login: ...` (exit 3)**
- Wrong user name or password: try signing in to the web client with the same credentials.
- The account uses single sign-on (Microsoft/Entra ID, ADFS, ...). SSO accounts cannot use the API; use a DocuWare
  user with a DocuWare password or an OAuth app ([DocuWare setup](docuware-setup.md)).
- The account is locked or has no licence.
- Client credentials: the app registration does not allow the client credentials grant, or the secret expired.

**`HTTP 404` during login**
The URL does not point at DocuWare. Cloud: use the tenant name only (`acme`). On-premises: use the address the web
client runs on (`https://dms.example.com`), without paths.

**TLS or certificate errors (on-premises)**
The server uses a certificate your OS does not trust. Install the company CA, or log in with `--insecure` if the
network is trusted.

**`warning: the password is stored unencrypted in .../secrets.json`**
No OS keyring was available (common on headless Linux and in containers) or `DW_SECRET_STORE=file` is set. Install and
unlock a Secret Service (GNOME Keyring, KWallet) and log in again, or accept the file with its `0600` permissions.

## Searching

**`search field "X" not found; available: ...` (exit 4)**
The field is not in this search dialog. Use a name from the list or from `dw fields <cabinet>`. Another dialog may have
it: `dw dialogs <cabinet>`, then `-d <dialog>`.

**`X is given twice; add --or ...` (exit 2)**
DocuWare cannot OR two values inside one condition. Add `--or`, or run one search per value. See
[Search](search.md#and-or-and-repeated-fields).

**`X is a number field; "..." is not a number` / `is a date field` (exit 2)**
Numbers use a dot for decimals and no thousands separator; dates are `YYYY-MM-DD`.

**`HTTP 422` (exit 1)**
DocuWare refused the query: a value it cannot parse, a function it does not allow on that field type, or (Cloud) more
than 10000 hits. Narrow the conditions.

**Search finds nothing although the document exists**
- The value must match exactly; use wildcards (`Peters*`).
- The user may lack rights on those documents.
- Metadata may be stale after a dialog change: add `--no-cache`.

**`HTTP 403` (exit 3)**
The account has no right for that cabinet or document.

**`HTTP 429`**
DocuWare Cloud's rate limit. dw already retried; wait a minute.

## Documents

**`no fulltext available` (exit 4)**
The cabinet is not fulltext-indexed or the document is not processed yet. Use `dw download` and read the file.

**Download has a different name than expected**
Existing files are never overwritten: `invoice (1).pdf` is used instead. `--force` overwrites. JSON output has the real
`path`.

**A multi-file document downloads as an archive**
That is how DocuWare delivers several sections. Pick one with `dw get` → `dw download --section <id>`, or use `--pdf`.

## Claude Code

**`/skills` does not list `docuware`**
Check that `~/.claude/skills/docuware/SKILL.md` (or the project's `.claude/skills/docuware/SKILL.md`) exists, then
restart Claude Code. Reinstall with `dw skill install`.

**Claude asks for permission on every `dw` call**
Add `"Bash(dw *)"` and `"PowerShell(dw *)"` to `permissions.allow` in `~/.claude/settings.json`
([Using dw from agents](agents.md#permissions)).

**Claude cannot find `dw` although it works in your terminal**
Claude Code was started before `dw` was put on the `PATH`. Restart it.

**Claude asks you for your DocuWare password**
It should not. Run `dw login` yourself in a terminal and tell Claude it is done.

## Still stuck

Run the failing command again with `--json` where possible and open an issue at
<https://github.com/rechedev9/docuware-cli/issues> with the command, the output and the DocuWare version (from
`dw status`). Remove company data first.
