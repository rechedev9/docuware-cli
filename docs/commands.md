---
title: Commands
description: "Reference for every dw command and flag."
---

# Commands

`dw <command> --help` prints the same information from the binary. JSON shapes are in [Output](output.md).

## Global flags

| Flag | Meaning |
|---|---|
| `--json` | machine-readable JSON on stdout |
| `-p, --profile <name>` | profile to use (default: the current profile, or `$DW_PROFILE`) |
| `--no-cache` | ignore cached cabinet and dialog metadata (same as `DW_NO_CACHE=1`) |
| `-v, --version` | print the version |
| `-h, --help` | help for any command |

Wherever a command takes `<cabinet>`, the file cabinet's name or id works (case-insensitive name match; unknown names
list the available ones).

## Session

### `dw login`

Sign in and save the connection as a profile. Details: [Authentication](authentication.md).

| Flag | Meaning |
|---|---|
| `--url <server>` | required: Cloud tenant name (`acme`), host name, or full URL (`https://dms.example.com`) |
| `-u, --user <name>` | DocuWare user name (OAuth2 password grant) |
| `--client-id <id>` | OAuth app client id instead of a user (client credentials grant) |
| `--password-stdin` | read the password or client secret from stdin instead of prompting |
| `--insecure` | skip TLS certificate checks (self-signed on-premises servers only) |
| `-p <name>` | profile name to save (default `default`); becomes the current profile |

Pass exactly one of `--user` or `--client-id`. Without `--password-stdin` the secret comes from `DW_PASSWORD` /
`DW_CLIENT_SECRET` if set, otherwise from a hidden prompt, which needs a real terminal.

```sh
dw login --url acme --user peggy.jenkins
dw login --url https://dms.example.com --user svc-reader --password-stdin < pw.txt
dw login --url acme --client-id 1b2c... -p service
```

### `dw logout`

Remove the current (or `-p`) profile, its stored secret and its cached tokens and metadata.

### `dw status`

Check the connection: server and version, account, organization, number of cabinets and trays, token expiry. Use it
first; exit code `3` means "not signed in".

## Metadata

### `dw cabinets` (alias `dw fc`)

List file cabinets. `--baskets` also lists document trays.

### `dw dialogs <cabinet>`

List the cabinet's dialogs (search, store, result list, ...) and mark the default search dialog.

### `dw fields <cabinet>`

Show the fields of the search dialog: database name, label, type, length and whether a value list exists.
`-d, --dialog <name or id>` picks another search dialog.

### `dw values <cabinet> <field>`

List the predefined values of a field (its select list). `-d, --dialog` as above.

## Documents

### `dw search <cabinet> [FIELD=VALUE ...]`

Search with the cabinet's search dialog. Syntax and rules: [Search](search.md).

| Flag | Meaning |
|---|---|
| `--or` | match any condition instead of all; required to repeat a field |
| `-s, --sort <FIELD[:asc\|:desc]>` | sort order, repeatable |
| `-n, --limit <N>` | maximum documents returned (default 20) |
| `--offset <N>` | skip N documents (paging; see `next_offset` in JSON) |
| `-d, --dialog <name or id>` | search dialog (default: the cabinet's default) |
| `-c, --columns <A,B,C>` | fields to show in text output |
| `--all-fields` | include DocuWare system fields |

```sh
dw search Invoices COMPANY=Peters* STATUS=Open
dw search Invoices "AMOUNT>=1000" INVOICE_DATE=2024-01-01..2024-12-31 --sort INVOICE_DATE:desc
dw search Invoices STATUS=Open STATUS=Overdue --or --limit 50 --json
```

### `dw get <cabinet> <id>`

Show one document: title, created/modified, every index field, and its sections (files) with ids, names, types,
pages and sizes. `--all-fields` includes system fields.

### `dw text <cabinet> <id>`

Print the text DocuWare extracted from the document (its "textshot"). The cabinet must be fulltext-indexed and the
document processed; otherwise exit code `4`.

| Flag | Meaning |
|---|---|
| `--max-chars <N>` | stop after N characters (default 20000; `0` = everything); a note goes to stderr when cut |
| `--section <id>` | only this section (see `dw get`) |

### `dw download <cabinet> <id>`

Save the document's file. A document with several sections arrives as a single archive unless `--section` picks one.

| Flag | Meaning |
|---|---|
| `-o, --output <path>` | file, directory, or `-` for stdout (default: current directory) |
| `--pdf` | convert to PDF |
| `--annotations` | burn annotations and stamps into the PDF |
| `--section <id>` | only this section |
| `--force` | overwrite an existing file (otherwise a numbered name is used) |

The file name comes from DocuWare, reduced to a safe base name. The printed path (or `path` in JSON) is where the file
went.

```sh
dw download Invoices 42
dw download Invoices 42 --pdf --annotations -o ./out/
dw download Invoices 42 -o - | pdftotext - -
```

## Everything else

### `dw api <path>`

GET any Platform REST path and print the raw JSON. The path is relative to `/DocuWare/Platform` unless it starts with
`/DocuWare/`; `href` values from responses can be passed back as they are. `-q key=value` adds query parameters
(repeatable). Only GET exists, so it cannot change data.

```sh
dw api Organizations
dw api FileCabinets/<cabinet-id>
dw api FileCabinets/<cabinet-id>/Query/Documents -q count=5
```

### `dw skill` / `dw skill install`

`dw skill` prints the embedded Claude Code skill. `dw skill install` writes it to `~/.claude/skills/docuware/SKILL.md`;
`--dir <skills dir>` writes to `<dir>/docuware/SKILL.md` instead (for example `.claude/skills` for one project).
See [Using dw from agents](agents.md).
