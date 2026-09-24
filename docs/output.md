---
title: Output and exit codes
description: "JSON shapes of every dw command, field value types, stderr conventions and exit codes."
---

# Output and exit codes

## Conventions

- **stdout** carries the result only. **stderr** carries `note:`, `warning:`, `error:` and `hint:` lines.
- `--json` works on every command. The output is a single JSON document: indented when stdout is a terminal,
  compact when piped. Key names are `snake_case` and stable.
- Text output is for people; its layout may change. Scripts and agents should use `--json`.

## Exit codes

| Code | Meaning | Typical causes |
|---|---|---|
| `0` | success | |
| `1` | other error | network failure, unexpected HTTP status, 422 from DocuWare |
| `2` | usage | unknown command or flag, missing argument, invalid search condition or sort |
| `3` | authentication | not logged in, login rejected, token rejected (401), no permission (403) |
| `4` | not found | unknown cabinet, dialog, field, profile or document; no fulltext for a document |

Errors look like this; the hint is optional:

```text
error: file cabinet "Nope" not found; available: Invoices, Contracts, Inbox
```

```text
error: not logged in to DocuWare
hint: run `dw login --url <server> --user <name>` in a terminal, or set DW_URL with DW_USERNAME/DW_PASSWORD (or DW_CLIENT_ID/DW_CLIENT_SECRET)
```

## Field values

Index fields are decoded from DocuWare's typed JSON into plain values:

| DocuWare type | JSON value | Example |
|---|---|---|
| Text, Memo | string | `"Peters Engineering"` |
| Int / Numeric | integer | `1001` |
| Decimal | number | `1250.5` |
| Date | `"YYYY-MM-DD"` | `"2024-01-15"` |
| DateTime | RFC 3339 | `"2024-01-15T09:00:00+01:00"` |
| Keywords | array of strings | `["urgent", "q1"]` |
| Table | array of objects, one per row | `[{"POS": 1, "ITEM": "Bolt"}]` |
| empty | `null` | `null` |

## status

```json
{"url":"https://acme.docuware.cloud","profile":"default","method":"password","account":"peggy",
 "credentials_from_env":false,"version":"7.12.0.0","organizations":["Peters Engineering"],
 "file_cabinets":2,"baskets":1,"token_expires":"2026-09-24T13:09:49+02:00"}
```

`method` is `password` or `client_credentials`. `credentials_from_env` is true when `DW_*` variables supplied them.

## login / logout

```json
{"profile":"default","url":"https://acme.docuware.cloud","method":"password","account":"peggy","version":"7.12.0.0","secret_store":"keyring"}
```

`secret_store` is `keyring` or `file`. `dw logout --json` prints `{"profile":"default","logged_out":true}`.

## cabinets

```json
[{"id":"fc-inv","name":"Invoices","basket":false,"default":true},
 {"id":"b-inbox","name":"Inbox","basket":true}]
```

Document trays (`basket: true`) only appear with `--baskets`.

## dialogs

```json
[{"id":"dlg-search","name":"Invoice search","type":"Search","default":true},
 {"id":"dlg-list","name":"Invoice list","type":"ResultList"}]
```

## fields

```json
{"cabinet":{"id":"fc-inv","name":"Invoices"},
 "dialog":{"id":"dlg-search","name":"Invoice search"},
 "fields":[{"name":"COMPANY","label":"Company","type":"Text","length":64},
           {"name":"STATUS","label":"Status","type":"Text","length":64,"select_list":true}]}
```

`select_list` is present when `dw values` can list predefined values.

## values

```json
{"field":"STATUS","values":["Open","Paid","Overdue"]}
```

## search

```json
{"cabinet":{"id":"fc-inv","name":"Invoices"},
 "total":3, "offset":0, "has_more":true, "next_offset":2,
 "items":[{"id":1,"title":"Peters Engineering",
           "fields":{"COMPANY":"Peters Engineering","INVOICE_DATE":"2024-01-15","AMOUNT":1250.5,
                     "INVOICE_NO":1001,"STATUS":"Paid","TAGS":["urgent","q1"],"CONTACT":null}}]}
```

- `total` is DocuWare's hit count; `next_offset` is absent when `has_more` is false.
- `fields` holds the result list's fields. Fields DocuWare flags as system fields (`DWDOCID`, `DWSTOREDATETIME`, ...)
  are included only with `--all-fields`.

## get

```json
{"id":1,"title":"Peters Engineering","cabinet":{"id":"fc-inv","name":"Invoices"},
 "created":"2024-01-15T09:00:00+01:00","modified":"2024-01-15T10:00:00+01:00",
 "content_type":"application/pdf","size":2048,
 "fields":{"COMPANY":"Peters Engineering","AMOUNT":1250.5,"TAGS":["urgent","q1"]},
 "sections":[{"id":"1-sec","file_name":"invoice-1001.pdf","content_type":"application/pdf","size":2048,"pages":2}]}
```

A section is one file of the document. Use its `id` with `dw text --section` or `dw download --section`.

## text

```json
{"cabinet":"Invoices","id":1,
 "sections":[{"id":"1-sec","file_name":"invoice-1001.pdf","text":"--- page 1 ---\nINVOICE 1001\n..."}]}
```

Multi-page text is separated by `--- page N ---` lines. When `--max-chars` cuts the text, stderr says
`note: text cut at N characters; use --max-chars 0 for everything`. A document without fulltext exits with `4`.

## download

```json
{"path":"C:\\Users\\peggy\\out\\invoice-1001 (1).pdf","bytes":24,"content_type":"application/pdf"}
```

`path` is where the file really went (numbered when the name was taken). With `-o -` the file goes to stdout and
nothing else is printed there.

## api

The raw JSON body of the response, unchanged except for indentation in a terminal.

## skill install

```json
{"path":"C:\\Users\\peggy\\.claude\\skills\\docuware\\SKILL.md"}
```
