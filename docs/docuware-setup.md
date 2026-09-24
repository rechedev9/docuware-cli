---
title: DocuWare setup
description: "What the DocuWare side needs for dw: version, a user or OAuth app, file cabinet rights, search dialogs and fulltext."
---

# DocuWare setup

Hand this page to whoever administers DocuWare. dw needs nothing installed on the server; it uses the standard
Platform REST API that every DocuWare system has.

## Checklist

- [ ] **DocuWare 7.10 or newer.** DocuWare Cloud is always current. On-premises: check the version under
      *About* in the web client, or run `dw status` after signing in.
- [ ] **An account for the API**, either:
  - a **DocuWare user with a DocuWare password** (not a single sign-on account: DocuWare does not allow SSO for the API),
    or
  - an **OAuth app registration** with a client secret and the client credentials grant enabled (see below).
- [ ] **Rights on the file cabinets** to be searched, through the user's roles or profiles.
- [ ] **A search dialog** assigned to that user on each cabinet, containing the fields people will search by.
- [ ] **Fulltext** on cabinets where `dw text` should work (optional).

## A dedicated read-only user

dw never changes documents, but a separate user keeps things auditable and limits what an agent can see:

1. Create a user (for example `api-reader`) with a DocuWare password.
2. Give it a role or profile with **search and view** rights on the relevant file cabinets and no store, edit or
   delete rights. `dw download` also needs whatever rights the profile requires to open or export files.
3. Assign the search dialogs it should use. The fields in the **search dialog** are what `dw fields` shows and what
   `dw search` can filter on; the **result list** decides which fields come back in search results.
4. Sign in once in the web client to confirm the password works.

A named user counts against DocuWare licences like any other user; check the licence model before creating one.

## OAuth app (client credentials)

For unattended use without a person's password:

1. In DocuWare Configuration, open *Integrations* → *App registration* (OAuth2 application registration).
2. Register a web application, create a **client secret**, and allow the **client credentials** grant.
3. Give the client id and secret to the person running `dw login --url <server> --client-id <id>` (the secret is
   typed at the prompt, never sent by email or chat).

Which DocuWare rights such a token has is decided by the app registration; test with `dw status` and `dw cabinets`.

## Fulltext

`dw text` returns the text DocuWare extracted with its fulltext/OCR service. It works only on cabinets with fulltext
enabled and only for documents that have already been processed. Without it, `dw text` exits with code `4` and
`dw download` still works.

## Rate limits (Cloud)

DocuWare Cloud limits some endpoints (sign-in, token, file cabinet list) to about 60 calls per minute and answers
`429` above that. dw caches tokens and cabinet metadata and retries `429` automatically, so normal use stays well
below the limit.

## Trying without a real system

DocuWare offers a free 30-day Cloud trial with sample data: <https://start.docuware.com/info/get-your-free-trial-ss>.
Its sample file cabinets are a good first target, and the [validation checklist](testing.md#validating-against-a-real-docuware)
says what to try. For development without any DocuWare, see [Testing](testing.md).
