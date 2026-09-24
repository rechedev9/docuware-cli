---
title: DocuWare API notes
description: "The Platform REST endpoints dw uses, the semantics it relies on, the evidence for each, and what still needs a real server to confirm."
---

# DocuWare API notes

dw was written against DocuWare's public documentation and reference clients, not against a live server. This page
records what it relies on and how sure we are, so anyone with a real system can confirm or correct it.

## Sources

- DocuWare's own Postman collection (2024-09-06), shipped in the laravel-docuware repository.
- The server's endpoint catalogue at `/DocuWare/Platform/Home/XSL` (a public on-premises 7.9 server).
- [DocuWare/PlatformJavaClient](https://github.com/DocuWare/PlatformJavaClient), DocuWare's REST-Sample-TS and
  Examples-dotNetCore.
- Responses recorded from a live DocuWare Cloud system in laravel-docuware's test fixtures.
- Support articles: [KBA-37505](https://support.docuware.com/en-us/knowledgebase/article/KBA-37505) (OAuth2),
  [KBA-36385](https://support.docuware.com/en-us/knowledgebase/article/KBA-36385) (rate limits),
  [KBA-36909](https://support.docuware.com/en-us/knowledgebase/article/KBA-36909) (10000-hit limit),
  [KBA-37482](https://support.docuware.com/en-us/knowledgebase/article/KBA-37482) (no SSO for the API).

## Conventions

- Base path `/DocuWare/Platform`, JSON with `Accept: application/json`, PascalCase keys.
- Hypermedia: resources carry `Links: [{rel, href}]`. dw follows links and falls back to the documented path only
  when a link is missing.
- Dates arrive as `/Date(<ms>)/`, optionally with an offset (`/Date(<ms>+0100)/`). `DateTime.MinValue` means empty.
- Lists are wrapped XML-style: `{"FileCabinet": [...]}`, `{"Dialog": [...]}`, `{"Keyword": [...]}`.
- Errors have `Message`, `Status`, `StatusCode`, `Uri`, `Method`; the token endpoint uses OAuth's `error` and
  `error_description`.

## Endpoints used

| Purpose | Request | Notes |
|---|---|---|
| Discovery | `GET /Home/IdentityServiceInfo` → `IdentityServiceUrl` | no auth |
| | `GET <identity>/.well-known/openid-configuration` → `token_endpoint` | no auth |
| Token | `POST <token_endpoint>` form: `grant_type=password`, `client_id=docuware.platform.net.client`, `scope=docuware.platform`, `username`, `password` | or `grant_type=client_credentials` with `client_id`/`client_secret` |
| Version | `GET /DocuWare/Platform` (service description, `Version`) | |
| Organizations | `GET /Organizations` → `{"Organization":[...]}` | |
| Cabinets | `GET /FileCabinets` → `{"FileCabinet":[{Id, Name, IsBasket, ...}]}` | rate-limited on Cloud; cached 1 h |
| Dialogs | `GET /FileCabinets/{id}/Dialogs` → `{"Dialog":[{"$type":"DialogInfo", ...}]}` | dialog `self` gives `Fields[]` and `Query.Links` |
| Select list | dialog field link `simpleSelectList` → `{"Value":[...]}` | |
| Search | `POST /FileCabinets/{id}/Query/DialogExpression?dialogId=&count=&start=` | via the dialog's `dialogExpression` link |
| List | `GET /FileCabinets/{id}/Query/Documents?count=&start=` | no conditions, no sort |
| Next page | result `Links` rel `next` | `first`/`prev` also exist |
| Document | `GET /FileCabinets/{id}/Documents/{docId}` | fields, sections |
| Download | document link `fileDownload`, fallback `/Documents/{docId}/FileDownload`; `targetFileType=Auto\|PDF`, `keepAnnotations` | section `fileDownload` for one file |
| Text | full section link `textshot` → `Pages[].Items[TextZone].Ln[].Items[Word].Value` | sections embedded in a document only have `self`; dw loads the section first |

Search body:

```json
{"Condition":[{"DBName":"COMPANY","Value":["Peters*"]}], "Operation":"And",
 "SortOrder":[{"Field":"INVOICE_DATE","Direction":"Desc"}]}
```

Result: `{"Items":[...], "Count":{"Value":123,"HasMore":true}, "Links":[...]}`.

## What dw relies on

| # | Assumption | Confidence | Evidence |
|---|---|---|---|
| 1 | A search without `fields` returns the result list's fields | high | catalogue: `fields` are "user index field names which will appear in addition to the result list fields" |
| 2 | Two values in one condition are a range (from, to); `null` is an open end | medium | .NET `Create(field, from, to)`, Java `create(fieldName, valueFrom, valueTo)`, Postman sample; the Postman text contradicts its own sample; `null` bound only from a community client |
| 3 | OR over values of one field = separate conditions with `"Operation":"Or"` | medium | documented pattern; the reason dw requires `--or` to repeat a field |
| 4 | `EMPTY()` / `NOTEMPTY()` and `*` / `?` wildcards | medium | community clients, .NET sample (`"T*"`); one KBA summary says `EMPTY()` is refused on number/date fields |
| 5 | Parentheses in values must be escaped with `\` | low | community client (422 otherwise) |
| 6 | Search dates are ISO `YYYY-MM-DD`; `/Date(ms)/` is refused | medium | Postman sample; community changelog (422) |
| 7 | `count`/`start` page both search and list; `Count` is `{Value, HasMore}` | high | catalogue, Postman, Java `CountPlusValue` |
| 8 | Cloud refuses more than 10000 hits per search (422) | high | KBA-36909 |
| 9 | Dialog `DWFieldType`: Text, Numeric, Decimal, Date, DateTime, Memo, Keyword, Table | high | Java `DWFieldType`, .NET `Document.cs` |
| 10 | Document `ItemElementName`: String, Int, Decimal, Date, DateTime, Memo, Keywords, Table | high | live Cloud fixture |
| 11 | `Date` values are shown in the local time zone | low | Python reference client; nothing official |
| 12 | Full sections have `textshot` and `fileDownload`; embedded ones only `self` | high | Java `Section.java`, live fixture |
| 13 | `Auto` download of a multi-section document returns an archive | low | undocumented |
| 14 | 429 with `Retry-After`, ~60/min on logon, token and `/FileCabinets` (Cloud) | high | KBA-36385 |
| 15 | OAuth2 password grant client `docuware.platform.net.client`, scope `docuware.platform`; SSO accounts cannot use it | high | KBA-37505, KBA-37482 |
| 16 | Which user a client-credentials token acts as | low | only the App Registration API's `allowClientCredentialsGrant` |

Points 2 to 6, 11, 13 and 16 are the ones to confirm first on a real server. The steps are in
[Testing](testing.md#validating-against-a-real-docuware).

## Not used yet

For the roadmap, from the same sources:

- Workflows: organization links `workflows` (→ `/Workflow/Workflows`, tasks at `/Workflow/Workflows/{id}/Tasks`),
  `controllerWorkflows`, `designerWorkflows`, `workflowRequests`. Task decisions are confirmed with
  `POST .../Decisions/{id}/Confirm`. Paths differ between variants, so follow links instead of building URLs.
- Search body extras: `AdditionalResultFields`, `AdditionalCabinets`, `ForceRefresh`, `IncludeSuggestions`.
- A document's `downloadAsArchive` link.
- Refresh tokens (absolute lifetime 0/30/60/90 days, sliding 15/30/45 days, per app registration).
