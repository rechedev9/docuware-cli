---
title: Architecture
description: "Goals, non-goals, package layout, request flow and security design of dw."
---

# Architecture

## Goals

- One static binary per platform, no runtime, no server component.
- Built for agents: `--json` everywhere, stable exit codes, errors that list valid choices, bounded output.
- Correct DocuWare semantics: search through the same dialogs the web client uses, validate conditions before sending
  them, decode typed field values.
- Safe by default: read-only, secrets in the OS keyring, tokens only to the configured host, no silent overwrites.

## Non-goals (for now)

- Writing to DocuWare (index updates, uploads, deletes, workflow decisions). When added, they will sit behind an
  explicit opt-in and confirmation.
- An MCP server. Claude Code and similar agents already run shell commands; a CLI plus a skill is less to install,
  easier to debug, and usable by people too.
- Interactive TUI features.

## Packages

| Package | Role |
|---|---|
| `cmd/dw` | entry point; sets the version from `-ldflags` or the module build info |
| `internal/cli` | cobra commands, flag parsing, text and JSON output, exit codes and hints |
| `internal/docuware` | REST client: discovery, OAuth2 grants, token renewal, retries, hypermedia links, search expressions, field decoding, textshot flattening, downloads |
| `internal/config` | profiles, keyring secrets with file fallback, token and metadata cache |
| `internal/dwfake` | in-memory DocuWare for tests; `cmd/dwfake` serves it for manual tries |
| `skill` | the Claude Code skill (`SKILL.md`), embedded with `go:embed` |

`internal/docuware` has no dependency on the CLI or config packages: config plugs in through the `Cache` interface and
the `OnToken` callback.

## Request flow

1. `cli.resolveSession` picks the profile (`-p`, `DW_PROFILE`, current, `default`) and applies `DW_*` overrides.
2. `cli.connect` builds a `docuware.Client` with the cached token, a callback that saves renewed tokens, and the
   1-hour metadata cache.
3. Every request goes through `Client.do`: it adds the bearer token (logging in first if needed), renews once on
   `401`, retries `429` up to 3 times honouring `Retry-After` (at most 30 s per wait), and turns other `4xx/5xx` into
   an `APIError` carrying DocuWare's `Message`.
4. Commands follow hypermedia links (`dialogExpression`, `next`, `fileDownload`, `textshot`, `simpleSelectList`) and
   fall back to documented paths when a link is absent.
5. `cli.classify` maps errors to exit codes and hints.

## Search

`docuware.BuildExpression` turns `FIELD=VALUE` arguments into a DialogExpression body. It resolves fields by database
name or label, validates numbers and dates, merges bounds into ranges, escapes parentheses, and refuses combinations
DocuWare would misread (a repeated field without `--or`, since two values in one condition mean a range). Without
conditions and sort it uses `/Query/Documents` instead. Paging follows `next` links up to `--limit` and computes
`has_more` from the total, or from the last page when the total is unknown.

## Security

- Secrets: OS keyring (`zalando/go-keyring`); file fallback `secrets.json` at `0600` with a warning. Never in
  `config.json`, never logged, never in error messages.
- Tokens are sent only to the configured host. Links pointing elsewhere are refused.
- Downloaded file names come from the server, so they are reduced to a safe base name (no paths, no reserved
  characters); existing files are kept unless `--force`, using exclusive creation.
- `--insecure` is per profile and must be chosen explicitly.
- `dw api` supports GET only.

## Dependencies

`spf13/cobra` (commands), `zalando/go-keyring` (keyring), `golang.org/x/term` (hidden prompt). Everything else is the
standard library. Go 1.26 is required (the version `x/term` needs).
