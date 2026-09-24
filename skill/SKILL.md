---
name: docuware
description: Search, read and download documents stored in DocuWare with the `dw` CLI. Use when the user asks about documents, invoices, contracts or other records kept in DocuWare, its file cabinets, index fields, or wants a DocuWare document's text or file.
---

# DocuWare via `dw`

`dw` talks to the DocuWare Platform REST API. It is read-only: it cannot change or delete anything.

## Workflow

1. `dw status` — confirms the login and shows what the account can see. Exit code 3 means not logged in: ask the user to run `dw login --url <tenant-or-host> --user <name>` in their own terminal (it prompts for the password; never ask them to paste it into the chat).
2. `dw cabinets` — file cabinets (add `--baskets` for document trays).
3. `dw fields <cabinet>` — searchable fields: database name, label, type, and whether a value list exists (`dw values <cabinet> <field>`). Always check fields before the first search in a cabinet; do not guess field names.
4. `dw search <cabinet> FIELD=VALUE ... --json` — find documents.
5. `dw get <cabinet> <id>` — all index fields and the files (sections) of one document.
6. `dw text <cabinet> <id>` — OCR fulltext, capped at 20000 characters by default (`--max-chars`).
7. `dw download <cabinet> <id> [-o dir/] [--pdf]` — save the file; prints the path.

Cabinet arguments accept the name or the id. Add `--json` to any command for structured output.

## Search conditions

| Syntax | Meaning |
|---|---|
| `COMPANY=Peters*` | exact match; `*` and `?` are wildcards (`*` for any text, `?` for one character) |
| `INVOICE_DATE=2024-01-01..2024-03-31` | range (number and date fields only) |
| `AMOUNT>=1000`, `AMOUNT<=5000` | open-ended bounds; quote them in the shell |
| `CONTACT=EMPTY()`, `CONTACT=NOTEMPTY()` | empty / non-empty field |
| `STATUS=Open STATUS=Overdue --or` | either value; a field may only repeat with `--or` |

- Conditions are ANDed; `--or` switches all of them to OR. DocuWare has no mixed AND/OR: for "(Open or Overdue) and Peters", run one search per status.
- Dates are `YYYY-MM-DD`. Decimals use a dot. Quote conditions containing `<`, `>` or spaces.
- Express every restriction as a condition. `--sort FIELD:desc` with `--limit` only orders and cuts the result.
- Paging: default `--limit 20`. JSON output has `total`, `has_more` and `next_offset`; pass `--offset <next_offset>` for the next page.
- A search without conditions lists the cabinet's documents as stored.

## Errors and exit codes

`0` ok · `1` other error · `2` bad usage or invalid condition · `3` authentication · `4` not found (unknown cabinet, field or document; no fulltext).
Errors go to stderr with a `hint:` line. Unknown cabinets and fields list the valid names, so read the error before retrying.

## Anything else

`dw api <path>` does a GET on any Platform path (relative to `/DocuWare/Platform`) and prints the raw JSON. Use it for resources without a command, such as organizations, workflows or users. DocuWare responses carry `Links` (`rel`/`href`); pass an `href` back to `dw api` to follow it.
