---
title: Releasing
description: "Maintainer checklist for cutting a dw release with GoReleaser and GitHub Actions."
---

# Releasing

Releases are built by GoReleaser in GitHub Actions (`.github/workflows/release.yml`) when a `v*` tag is pushed.

## What a release contains

- Archives `dw_<version>_<os>_<arch>` for windows, darwin and linux on amd64 and arm64: `.zip` for Windows, `.tar.gz`
  otherwise. Each holds `dw` (`dw.exe`), `README.md` and `LICENSE`.
- `checksums.txt` with SHA-256 sums.
- Release notes generated from the commits since the previous tag.
- Binaries are built with `CGO_ENABLED=0` and `-X main.version=<version>`. They are not code-signed or notarized.

## Checklist

1. `main` is green in CI.
2. `go vet ./... && go test ./...` locally.
3. Move the `Unreleased` entries in `CHANGELOG.md` under the new version with today's date; commit.
4. If search syntax, commands or exit codes changed, check `skill/SKILL.md`, `docs/commands.md`, `docs/output.md`
   and `docs/search.md`.
5. Tag and push:

   ```sh
   git tag -a v0.1.0 -m "dw v0.1.0"
   git push origin v0.1.0
   ```

6. Watch the run: `gh run watch` (or the Actions tab).
7. Check the release page has 6 archives and `checksums.txt`, then run the install steps from
   [Agent setup runbook](agent-setup.md#2a-prebuilt-release) on at least one OS and confirm `dw --version`.

## Versioning

Semantic versioning. Until 1.0, minor versions may change flags or JSON output; each change is listed in the
changelog. Once a command, flag, JSON key or exit code has shipped in a tagged release, treat it as a public contract.
