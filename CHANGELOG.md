# Changelog

All notable changes to this project are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## Unreleased

First version.

### Added

- `dw login`, `logout`, `status`: OAuth2 password and client-credentials grants with discovery through
  `IdentityServiceInfo`, profiles, secrets in the OS keyring (file fallback), cached and self-renewing tokens,
  `DW_*` environment overrides for CI.
- `dw cabinets`, `dialogs`, `fields`, `values`: file cabinet and search dialog metadata, cached for one hour.
- `dw search`: conditions validated against the search dialog (`=`, wildcards, `FROM..TO`, `>=`, `<=`, `EMPTY()`,
  `NOTEMPTY()`), `--or`, `--sort`, paging with `total`/`has_more`/`next_offset`.
- `dw get`, `text`, `download`: index fields and sections, OCR fulltext capped by `--max-chars`, downloads as stored
  or PDF that never overwrite unless `--force`.
- `dw api`: raw GET on any Platform path.
- `dw skill` / `dw skill install`: the Claude Code skill, embedded in the binary.
- `--json` on every command; exit codes 0/1/2/3/4 with `hint:` lines.
- Retries on HTTP 429 (honouring `Retry-After`) and one token renewal on 401.
- Documentation for people and agents under `docs/`, including an agent setup runbook.

### Notes

- A field may repeat in `dw search` only with `--or`: DocuWare reads two values in one condition as a range, so each
  value becomes its own OR condition.
- Result pages are requested with at most 10000 documents (DocuWare Cloud's limit).
