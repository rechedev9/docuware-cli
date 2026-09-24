---
title: Testing
description: "Unit and end-to-end tests against an in-memory DocuWare, trying dw without a server, and the checklist for validating against a real DocuWare."
---

# Testing

## Automated tests

```sh
go vet ./...
go test ./...
```

CI runs both on Linux, Windows and macOS for every push and pull request (`.github/workflows/ci.yml`).

The tests never touch a real DocuWare. They run against `internal/dwfake`, an in-memory server whose responses follow
the shapes in [DocuWare API notes](docuware-api.md): identity discovery, both OAuth2 grants, cabinets and a basket,
dialogs, a select list, three documents, paging with `next` links, textshots, downloads, `401` for expired tokens and
`429` throttling.

| Test file | Covers |
|---|---|
| `internal/docuware/client_test.go` | login and token cache, renewal on 401, bad logins, client credentials, 429 retries, refusing foreign hosts, cabinet/dialog lookup, search expressions and paging, OR, text, downloads, file name sanitizing, sections, select lists, metadata cache |
| `internal/docuware/search_test.go` | condition parsing and validation, value decoding, `Count` shapes, `SanitizeFileName` |
| `internal/cli/cli_test.go` | the binary's commands end to end: login and profiles, environment credentials, every command's output, exit codes, logout, `api`, `skill install` |

Rules for new code: a new endpoint gets a handler in `dwfake` modelled on a documented response (note the source in a
comment), and a test that goes through it.

## Trying dw without DocuWare

```sh
go run ./internal/dwfake/cmd/dwfake
# fake DocuWare at http://127.0.0.1:51973 (user peggy / s3cret, client svc-app / svc-secret)
```

In another terminal, with a throwaway config so your real profiles stay untouched:

```sh
export DW_CONFIG_DIR=/tmp/dw-try DW_CACHE_DIR=/tmp/dw-try-cache DW_SECRET_STORE=file
echo s3cret | dw login --url http://127.0.0.1:51973 --user peggy --password-stdin
dw cabinets
dw fields Invoices
dw search Invoices COMPANY=Peters* --json
dw text Invoices 1
```

PowerShell: `$env:DW_CONFIG_DIR = "$env:TEMP\dw-try"` and so on.

## Validating against a real DocuWare

A free 30-day Cloud trial works: <https://start.docuware.com/info/get-your-free-trial-ss>. Each step names the
assumption from [DocuWare API notes](docuware-api.md#what-dw-relies-on) it confirms. Report results (including
"works") in an issue.

1. **Sign in.** `dw login --url <tenant> --user <user>`, then `dw status`. Confirms discovery and the password grant (15).
2. **Metadata.** `dw cabinets`, `dw dialogs <cabinet>`, `dw fields <cabinet>`. Note the types shown (9).
3. **List.** `dw search <cabinet> --limit 3 --json`: `total` is set and `items` have fields (1, 7).
4. **Text condition and wildcard.** `dw search <cabinet> <TEXTFIELD>=<start>*` (4).
5. **Range.** `dw search <cabinet> "<DATEFIELD>=2020-01-01..2030-12-31"` and `"<NUMBERFIELD>>=1"` return documents
   inside the range only (2, 6).
6. **OR on one field.** `dw search <cabinet> <FIELD>=<a> <FIELD>=<b> --or` returns documents with either value (3).
7. **Empty checks.** `<TEXTFIELD>=EMPTY()` and, separately, `<DATEFIELD>=EMPTY()` (4; the second may be refused).
8. **Parentheses.** A value with `(`, for example a company named `X (Y)` (5).
9. **Paging.** On a cabinet with more than 30 documents: `dw search <cabinet> --limit 30 --json`, then
   `--offset <next_offset>`. No duplicates or gaps (7).
10. **Dates.** `dw get <cabinet> <id>`: date fields show the same day as the web client (11).
11. **Field types.** A document with keywords, a memo and a table field, if the system has them (10).
12. **Text.** `dw text <cabinet> <id>` on a fulltext-indexed cabinet matches the document (12).
13. **Download.** `dw download <cabinet> <id>`, `--pdf`, and a document with several files, with and without
    `--section` (12, 13).
14. **Client credentials** (only if an OAuth app exists): `dw login --url <tenant> --client-id <id>`, then
    `dw cabinets`. Which cabinets appear tells which user the token acts as (16).

`dw api <path>` shows the raw JSON whenever a result looks wrong; include it (without company data) in the report.
