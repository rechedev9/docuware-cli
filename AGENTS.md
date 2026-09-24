# AGENTS.md

Instructions for AI agents working in this repository.

**Were you asked to install or set up `dw` for someone, not to change its code?** Stop here and follow
[docs/agent-setup.md](docs/agent-setup.md).

## Commands

```sh
go build ./...
go vet ./...
go test ./...
gofmt -l .                                # must print nothing
go run ./cmd/dw --help
go run ./internal/dwfake/cmd/dwfake       # fake DocuWare on a random local port
```

Go 1.26+ (required by `golang.org/x/term`). All files use LF line endings (`.gitattributes`).

## Package map

- `cmd/dw`: entry point and version stamping.
- `internal/cli`: cobra commands, output formatting, exit codes and hints (`classify` in `root.go`).
- `internal/docuware`: the REST client. Auth and discovery (`auth.go`), transport and retries (`client.go`),
  endpoints (`api.go`), search expressions (`search.go`), field decoding (`values.go`), OCR text (`textshot.go`).
- `internal/config`: profiles, keyring secrets, token and metadata cache.
- `internal/dwfake`: in-memory DocuWare used by every test.
- `skill`: `SKILL.md`, embedded into the binary and installed by `dw skill install`.
- `scripts/header.py`: renders the README banner `docs/assets/header.png` (`python scripts/header.py`, needs Pillow and numpy).

Architecture and design rationale: [docs/architecture.md](docs/architecture.md).

## Rules

- **Read-only.** No command may send anything but GET to DocuWare, except the token request and search POSTs. Write
  features need an explicit design (opt-in flag, confirmation) agreed first.
- **Secrets.** Never print, log or put passwords, client secrets or tokens in errors. Never add a flag that takes a
  secret as an argument value.
- **Hosts.** The bearer token goes only to the configured host (`Client.resolve`). Keep it that way.
- **Evidence first.** DocuWare behaviour must come from a source: official docs, KBAs, DocuWare's GitHub samples,
  the Platform endpoint catalogue, or recorded responses. Record it in [docs/docuware-api.md](docs/docuware-api.md)
  with a confidence level, and model the response in `internal/dwfake` before relying on it.
- **Tests.** Every change gets a test through `dwfake` (client level) or through `cli.Run` (command level). Tests must
  not need network access or a keyring (`DW_SECRET_STORE=file`, temporary `DW_CONFIG_DIR`/`DW_CACHE_DIR`).
- **Errors.** Wrong input from the user or agent returns a `QueryError`/`usageError` (exit 2) with a message that says
  how to fix it; "not found" errors list the valid names (`NotFoundError.Available`).
- **Output.** `--json` output is one JSON document on stdout, `snake_case` keys. Notes and warnings go to stderr.

## Public contracts

Commands, flags, JSON keys, exit codes, the search syntax and `SKILL.md` are public contracts once they ship in a
tagged release: agents depend on them. Change them only with a `CHANGELOG.md` entry. Anything that has not been in a
tagged release yet may still change.

## When you change behaviour

Update in the same change:

- `skill/SKILL.md` if agents should use dw differently (keep it short; it is loaded into agent context);
- `docs/commands.md`, `docs/search.md`, `docs/output.md` for commands, syntax and JSON;
- `CHANGELOG.md` under `Unreleased`;
- the command's `Long`/`Example` help text.

## Releasing

See [docs/releasing.md](docs/releasing.md). Tags and releases are published only when the maintainer asks.
