# dw — DocuWare from the terminal

`dw` is a single-binary CLI for the DocuWare Platform REST API (Cloud or on-premises, 7.10+).
It is built to be driven by coding agents such as Claude Code: `--json` everywhere, stable exit codes,
errors that list the valid choices, and output capped to keep agent context small.

It is read-only for now: it searches, reads and downloads, but cannot change or delete anything.

## Install

Download the archive for your system from [Releases](https://github.com/rechedev9/docuware-cli/releases) when one is published,
unpack `dw` (`dw.exe` on Windows) into a folder on your `PATH`, and check with `dw --version`.

With Go 1.26+ installed you can build it instead:

```sh
go install github.com/rechedev9/docuware-cli/cmd/dw@latest   # installs into ~/go/bin (%USERPROFILE%\go\bin)
```

## Use with Claude Code

1. **Log in once, in a normal terminal** (not through Claude Code: its shell cannot answer the hidden password prompt):

   ```sh
   dw login --url <tenant> --user <docuware-user>
   dw status
   ```

2. **Install the skill**, which tells Claude when and how to use `dw`:

   ```sh
   dw skill install                          # user-wide: ~/.claude/skills/docuware/SKILL.md
   dw skill install --dir .claude/skills     # or only for one project
   ```

   Restart Claude Code and check that `/skills` lists `docuware`. Claude then picks it up on its own when you ask about
   DocuWare documents, or you can call it with `/docuware`.

3. **Optional: skip the permission prompt for every `dw` call.** `dw` cannot change data, so allowing it is low risk.
   In `~/.claude/settings.json` (or a project's `.claude/settings.json`):

   ```json
   {
     "permissions": {
       "allow": ["Bash(dw *)", "PowerShell(dw *)"]
     }
   }
   ```

No MCP server or other Claude Code configuration is needed. Then just ask, for example:
"Find the open invoices from Peters Engineering in DocuWare from this year and summarize the biggest one."

### What the DocuWare side needs

- DocuWare 7.10 or newer (Cloud is always current).
- A DocuWare user that can sign in with a DocuWare password and has rights on the file cabinets to search
  (its search dialogs define which fields are searchable). A dedicated read-only user is a good idea.
- Or, for unattended use, an OAuth application registered in DocuWare with a client secret; log in with `--client-id`.

## Sign in

```sh
dw login --url acme --user peggy.jenkins        # Cloud tenant "acme"; prompts for the password
dw login --url https://dms.example.com --user svc --password-stdin < pw.txt
dw login --url acme --client-id <id> --password-stdin -p service   # OAuth2 client credentials
```

- Passwords and client secrets go to the OS keyring (Windows Credential Manager, macOS Keychain, Secret Service).
  Profiles live in the user config dir (`dw/config.json`), tokens and metadata in the user cache dir.
- Access tokens (60 min) are cached and renewed automatically. The token endpoint is remembered, so later runs skip discovery.
- For CI or one-off runs, set `DW_URL` plus `DW_USERNAME`/`DW_PASSWORD` or `DW_CLIENT_ID`/`DW_CLIENT_SECRET`; they override the profile.
- Other variables: `DW_PROFILE`, `DW_INSECURE=1` (self-signed on-prem only), `DW_NO_CACHE=1`,
  `DW_CONFIG_DIR`, `DW_CACHE_DIR`, `DW_SECRET_STORE=file` (no keyring available).

## Use

```sh
dw status                                   # who am I, what can I see
dw cabinets [--baskets]
dw fields Invoices                          # searchable fields: name, label, type, value list
dw values Invoices STATUS
dw search Invoices COMPANY=Peters* "AMOUNT>=1000" INVOICE_DATE=2024-01-01..2024-12-31 --sort INVOICE_DATE:desc
dw get Invoices 42
dw text Invoices 42 [--max-chars 0]
dw download Invoices 42 [--pdf] [--annotations] [-o dir/ | -o -]
dw api FileCabinets/<id>                    # raw GET for anything without a command
```

Search syntax: `FIELD=VALUE` (with `*`/`?` wildcards), `FIELD=FROM..TO`, `FIELD>=V`, `FIELD<=V`, `FIELD=EMPTY()`, `FIELD=NOTEMPTY()`.
Repeating a text field ORs its values. `--or` combines conditions with OR. See `dw search --help`.

Exit codes: `0` ok, `1` error, `2` usage or invalid condition, `3` authentication, `4` not found.

## Design

| Package | Role |
|---|---|
| `internal/docuware` | REST client: OAuth2 discovery (IdentityServiceInfo → OpenID configuration → token), password and client-credentials grants, token renewal on 401, retries on 429, hypermedia links with documented fallbacks, search expressions, field decoding (`/Date(ms)/`, keywords, tables), textshot flattening |
| `internal/config` | profiles, keyring secrets, token and metadata cache (1 h TTL) |
| `internal/cli` | cobra commands and output |
| `internal/dwfake` | in-memory DocuWare for tests; `go run ./internal/dwfake/cmd/dwfake` serves it to try `dw` without a server |
| `skill` | the Claude Code skill, embedded in the binary |

Security details: the bearer token is only ever sent to the configured host, even when a response links elsewhere;
server-supplied file names are reduced to safe base names; downloads never overwrite files unless `--force`.

## Tests

```sh
go test ./...
```

The tests run against `internal/dwfake`, whose responses follow the official DocuWare REST samples and the JSON
the community clients parse. They cannot prove behaviour of a real server.

## To validate against a real DocuWare

A free 30-day DocuWare Cloud trial (with sample data) works: https://start.docuware.com/info/get-your-free-trial-ss

These points are implemented from documentation and reference clients and need a real server to confirm:

1. Searches send no `fields` parameter and expect the result list's fields back.
2. Several values on one text field are ORed; two values on a number or date field form a range.
3. A search without conditions uses `GET /FileCabinets/{id}/Query/Documents`, and `count`/`start` page every query.
4. `Date` fields are read in the local time zone (as the Python reference client does).
5. Section fulltext is found through the `textshot` link, loading the section when the document omits it.
6. Search dialog field types (`DWFieldType`) may use `Text`/`Numeric` or `String`/`Int`; both are handled.

## Roadmap

- Writes behind explicit opt-in and confirmation: update index fields, upload, delete.
- Workflow tasks: list, show, confirm decisions (`workflows` → `tasks` → decision links).
- Authorization Code + PKCE login for accounts that cannot use the password grant.
