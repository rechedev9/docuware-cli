<p align="center">
  <img src="docs/assets/header.png" alt="dw: DocuWare from the terminal. Mount Fuji, a red sun and waves in ukiyo-e style." width="100%">
</p>

# dw 🗂️ — DocuWare from the terminal

Search, read and download DocuWare documents with one binary, built so Claude Code and other agents can use it
safely.

`dw` talks to the DocuWare Platform REST API (Cloud or on-premises, 7.10+). Every command has `--json`, exit codes are
stable, errors list the valid choices, and output is capped to keep agent context small. It is **read-only**: nothing
it does can change or delete a document.

## Set it up with your AI agent

Paste this into Claude Code (or any coding agent that can run shell commands):

```text
Install and set up the DocuWare CLI "dw" for me by following
https://raw.githubusercontent.com/rechedev9/docuware-cli/main/docs/agent-setup.md step by step.
```

The agent installs the binary, asks you for your DocuWare address and user name, has **you** type the password in
your own terminal (it never sees it), installs the Claude Code skill and checks everything works.

## Install

Prebuilt binaries for Windows, macOS and Linux: [latest release](https://github.com/rechedev9/docuware-cli/releases/latest).
Unpack `dw` (`dw.exe`) into a folder on your `PATH`.

With Go 1.26+:

```sh
go install github.com/rechedev9/docuware-cli/cmd/dw@latest
```

More options and platform notes: [docs/install.md](docs/install.md).

## Quickstart

```sh
dw login --url acme --user peggy.jenkins     # Cloud tenant "acme", or --url https://dms.example.com
dw status                                    # server, account, what you can see
dw cabinets
dw fields Invoices                           # searchable fields and their types
dw search Invoices COMPANY=Peters* "AMOUNT>=1000" INVOICE_DATE=2024-01-01..2024-12-31 --sort INVOICE_DATE:desc
dw get Invoices 42                           # index fields and files
dw text Invoices 42                          # OCR fulltext
dw download Invoices 42 --pdf -o ./out/
```

Run `dw login` in a normal terminal: the password prompt needs one. The password is stored in the OS keyring
(Windows Credential Manager, macOS Keychain, Secret Service) and tokens renew by themselves.

## Use with Claude Code

```sh
dw skill install          # writes ~/.claude/skills/docuware/SKILL.md
```

Restart Claude Code, check `/skills` lists `docuware`, and ask in plain language, e.g. *"Find the open invoices from
Peters Engineering in DocuWare and summarize the biggest one."* No MCP server is needed.

Optional, to skip the permission prompt on every call (`dw` cannot change data), in `~/.claude/settings.json`:

```json
{ "permissions": { "allow": ["Bash(dw *)", "PowerShell(dw *)"] } }
```

Details: [docs/agent-usage.md](docs/agent-usage.md).

## What DocuWare needs

- DocuWare 7.10 or newer (Cloud is always current).
- A DocuWare user with a DocuWare password and rights on the file cabinets. Single sign-on accounts cannot use the
  API. A dedicated read-only user is best.
- Or an OAuth app registration with a client secret, for unattended use (`dw login --client-id`).

Checklist for the administrator: [docs/docuware-setup.md](docs/docuware-setup.md).

## Commands

| Command | Does |
|---|---|
| `dw login` / `logout` / `status` | sign in, sign out, check the connection |
| `dw cabinets` | list file cabinets (`--baskets` for trays) |
| `dw dialogs <cabinet>` | list dialogs |
| `dw fields <cabinet>` | searchable fields: name, label, type, value list |
| `dw values <cabinet> <field>` | predefined values of a field |
| `dw search <cabinet> [FIELD=VALUE ...]` | search; `--or`, `--sort`, `--limit`, `--offset` |
| `dw get <cabinet> <id>` | one document's fields and files |
| `dw text <cabinet> <id>` | OCR text, capped at 20000 characters |
| `dw download <cabinet> <id>` | save the file (`--pdf`, `--section`, `-o`) |
| `dw api <path>` | raw GET on any Platform path |
| `dw skill [install]` | print or install the Claude Code skill |

Search conditions: `FIELD=VALUE` (wildcards `*` `?`), `FIELD=FROM..TO`, `FIELD>=V`, `FIELD<=V`, `FIELD=EMPTY()`,
`FIELD=NOTEMPTY()`. A field may repeat only with `--or`. Exit codes: `0` ok, `1` error, `2` usage, `3` auth,
`4` not found. Full reference: [docs/commands.md](docs/commands.md), [docs/search.md](docs/search.md),
[docs/output.md](docs/output.md).

## Documentation

| Page | For |
|---|---|
| [Agent setup runbook](docs/agent-setup.md) | an AI agent installing dw for someone |
| [Install](docs/install.md) · [Quickstart](docs/quickstart.md) | first steps |
| [Using dw from agents](docs/agent-usage.md) | Claude Code skill, permissions, calling patterns |
| [DocuWare setup](docs/docuware-setup.md) | the DocuWare administrator |
| [Search](docs/search.md) · [Commands](docs/commands.md) · [Output](docs/output.md) | reference |
| [Authentication](docs/authentication.md) · [Configuration](docs/configuration.md) | sign-in, profiles, files, environment |
| [Troubleshooting](docs/troubleshooting.md) | when something fails |
| [Architecture](docs/architecture.md) · [DocuWare API notes](docs/docuware-api.md) · [Testing](docs/testing.md) · [Releasing](docs/releasing.md) | contributors |

## Status

The tests run against an in-memory DocuWare built from DocuWare's official samples, reference clients and recorded
Cloud responses; CI runs them on Linux, Windows and macOS. dw has not yet been run against a live DocuWare. What that
still has to confirm is listed in [docs/docuware-api.md](docs/docuware-api.md#what-dw-relies-on), with a checklist in
[docs/testing.md](docs/testing.md#validating-against-a-real-docuware). Reports welcome.

Roadmap: writes behind explicit opt-in (index fields, upload, delete), workflow tasks, Authorization Code + PKCE login.

## Development

```sh
go test ./...
go run ./internal/dwfake/cmd/dwfake     # a fake DocuWare to try dw against
```

Contributors and coding agents: read [AGENTS.md](AGENTS.md) first.

## License

MIT. Not affiliated with or endorsed by DocuWare GmbH.
