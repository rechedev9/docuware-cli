---
title: Quickstart
description: "From a fresh install to searching DocuWare from Claude Code in five minutes."
---

# Quickstart

You need a DocuWare user that signs in with a DocuWare password (not single sign-on) and has rights on at least one
file cabinet. Unsure? See [DocuWare setup](docuware-setup.md).

## 1. Install

Follow [Install](install.md), then check:

```sh
dw --version
```

## 2. Sign in

In a normal terminal window (the password prompt needs one):

```sh
dw login --url acme --user peggy.jenkins
```

`--url` is your Cloud tenant name (`acme` for `https://acme.docuware.cloud`) or your server's address
(`https://dms.example.com`). The password goes to the OS keyring; you will not be asked again.

```sh
dw status
```

```text
Server:    https://acme.docuware.cloud (DocuWare 7.12.0.0)
Account:   peggy.jenkins (password, from profile "default")
Org:       Peters Engineering
Cabinets:  2 file cabinets, 1 document tray
Token:     valid until 2026-09-24 13:09
```

## 3. Look around

```sh
dw cabinets
dw fields Invoices
```

```text
FIELD         LABEL         TYPE     LENGTH  LIST
COMPANY       Company       Text     64
INVOICE_DATE  Invoice date  Date     64
AMOUNT        Amount        Decimal  64
STATUS        Status        Text     64      yes
```

## 4. Search and read

```sh
dw search Invoices COMPANY=Peters* "AMOUNT>=1000" --sort INVOICE_DATE:desc
dw get Invoices 1
dw text Invoices 1
dw download Invoices 1 --pdf -o ./out/
```

The condition syntax is in [Search](search.md).

## 5. Hand it to Claude Code

```sh
dw skill install
```

Restart Claude Code and check that `/skills` lists `docuware`. Then ask in plain language:

> Find the open invoices from Peters Engineering in DocuWare from this year and summarize the biggest one.

Optional, to stop the permission prompt on every call: add `"Bash(dw *)"` and `"PowerShell(dw *)"` to
`permissions.allow` in `~/.claude/settings.json`. Details: [Using dw from agents](agents.md).
