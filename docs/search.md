---
title: Search
description: "Condition syntax, field types, AND/OR rules, sorting and paging for dw search."
---

# Search

```sh
dw search <cabinet> [FIELD=VALUE ...] [--or] [--sort FIELD[:asc|:desc]] [--limit N] [--offset N] [--json]
```

`dw search` runs a search through one of the cabinet's **search dialogs**, exactly like DocuWare's web client. The dialog
decides which fields can be searched, so start with:

```sh
dw fields Invoices
```

```text
Invoices - search dialog "Invoice search"

FIELD         LABEL         TYPE     LENGTH  LIST
COMPANY       Company       Text     64
INVOICE_DATE  Invoice date  Date     64
AMOUNT        Amount        Decimal  64
INVOICE_NO    Invoice no.   Numeric  64
STATUS        Status        Text     64      yes
```

`FIELD` is the database name; the label works too (`"Invoice date=2024-01-15"`), case-insensitively. Fields marked
`LIST` have predefined values: `dw values Invoices STATUS`.

A cabinet can have several search dialogs with different fields. `dw dialogs <cabinet>` lists them and `-d <name or id>`
picks one for `fields`, `values` and `search`; the default is the cabinet's default search dialog.

## Conditions

| Condition | Meaning | Field types |
|---|---|---|
| `COMPANY=Peters Engineering` | equal to | all |
| `COMPANY=Peters*` | wildcard: `*` any text, `?` one character | text |
| `INVOICE_DATE=2024-01-01..2024-03-31` | range, both ends included | number, date |
| `AMOUNT>=1000` | lower bound | number, date |
| `AMOUNT<=5000` | upper bound | number, date |
| `AMOUNT>=1000 AMOUNT<=5000` | bounds on the same field merge into one range | number, date |
| `INVOICE_NO=..1500` / `INVOICE_NO=1000..` | open range | number, date |
| `CONTACT=EMPTY()` | the field has no value | all |
| `CONTACT=NOTEMPTY()` | the field has a value | all |

Values:

- **Dates** are `YYYY-MM-DD`. Date-time fields also accept `YYYY-MM-DDTHH:MM:SS`.
- **Numbers** use a dot for decimals (`12.5`); no thousands separators.
- **Text** is passed as typed. `..` inside text is literal. Parentheses are escaped for you, so `COMPANY=Acme (EU)`
  searches for the literal text rather than a DocuWare function.
- Quote any condition with spaces, `<` or `>` for your shell: `"AMOUNT>=1000"`, `"COMPANY=Peters Engineering"`.

Invalid conditions are rejected before anything is sent, with exit code `2` and a message naming the problem:

```text
error: AMOUNT is a number field; "abc" is not a number (use a dot for decimals)
hint: run `dw fields <cabinet>` to see field names and types
```

## AND, OR and repeated fields

All conditions are combined with **AND**. `--or` combines **all** of them with OR instead. DocuWare's search
expressions have a single operator, so there is no mixed `(A or B) and C`.

A field may appear more than once only with `--or`:

```sh
dw search Invoices STATUS=Open STATUS=Overdue --or        # Open or Overdue
dw search Invoices STATUS=Open STATUS=Overdue             # error: STATUS is given twice; add --or ...
```

The reason: DocuWare reads two values in one condition as a range (from, to), so "Open or Overdue" cannot be sent as a
single condition. With `--or`, each value becomes its own condition. For "(Open or Overdue) and company Peters",
run two searches:

```sh
dw search Invoices STATUS=Open COMPANY=Peters*
dw search Invoices STATUS=Overdue COMPANY=Peters*
```

Bounds are the exception: `AMOUNT>=1000 AMOUNT<=5000` always merges into one range, with or without `--or`.

## Sorting

```sh
dw search Invoices STATUS=Open --sort INVOICE_DATE:desc --sort AMOUNT
```

`--sort` repeats; each entry is `FIELD`, `FIELD:asc` or `FIELD:desc`. Names not in the dialog pass through unchanged,
so system fields such as `DWSTOREDATETIME` (stored on) work.

## Limits and paging

- `--limit` (default 20) is the most documents returned; `--offset` skips documents.
- dw follows DocuWare's `next` links until it has `--limit` documents, and never asks for more than 10000 per page.
- JSON output tells you what is left:

  ```json
  {"total": 3, "offset": 0, "has_more": true, "next_offset": 2, "items": [ ... ]}
  ```

  Run the same search with `--offset 2` for the next page. `next_offset` is absent when nothing is left.
- DocuWare Cloud refuses searches with more than 10000 hits (HTTP 422). Narrow the conditions.
- A search **without conditions** lists the cabinet's documents in DocuWare's default order. It uses
  `/Query/Documents` and needs no search dialog; adding `--sort` switches back to the dialog.

## Output columns

Text output shows up to eight fields of the result list. Pick columns with `-c COMPANY,AMOUNT,STATUS`. Fields DocuWare flags as system fields
(`DWDOCID`, `DWSTOREDATETIME`, ...) are hidden unless `--all-fields`. JSON output always has every returned field; see
[Output](output.md#search).

## What is sent

For reference, `dw search Invoices COMPANY=Peters* "AMOUNT>=1000" --sort INVOICE_DATE:desc` posts this to
`/DocuWare/Platform/FileCabinets/<id>/Query/DialogExpression?dialogId=<dialog>&count=20&start=0`:

```json
{
  "Condition": [
    {"DBName": "COMPANY", "Value": ["Peters*"]},
    {"DBName": "AMOUNT", "Value": ["1000", null]}
  ],
  "Operation": "And",
  "SortOrder": [{"Field": "INVOICE_DATE", "Direction": "Desc"}]
}
```

More about the endpoints and how they were verified: [DocuWare API notes](docuware-api.md).
