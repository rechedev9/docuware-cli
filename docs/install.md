---
title: Install
description: "Install dw from a prebuilt release, with go install, or from source, on Windows, macOS and Linux."
---

# Install

`dw` is a single executable with no dependencies. Pick one way in. Agents: the
[Agent setup runbook](agent-setup.md#2-install-the-binary) has copy-paste scripts with checksum verification.

## Prebuilt release

Download the archive for your system from the
[latest release](https://github.com/rechedev9/docuware-cli/releases/latest):

| System | Archive |
|---|---|
| Windows (most PCs) | `dw_<version>_windows_amd64.zip` |
| Windows on ARM | `dw_<version>_windows_arm64.zip` |
| macOS, Apple Silicon | `dw_<version>_darwin_arm64.tar.gz` |
| macOS, Intel | `dw_<version>_darwin_amd64.tar.gz` |
| Linux x86-64 / ARM64 | `dw_<version>_linux_amd64.tar.gz` / `dw_<version>_linux_arm64.tar.gz` |

Unpack it and put `dw` (`dw.exe`) in a folder on your `PATH`:

- **Windows:** for example `%LOCALAPPDATA%\Programs\dw`. Add the folder via *Settings → System → About → Advanced
  system settings → Environment Variables → Path (user)*, then open a new terminal.
- **macOS / Linux:** for example `~/.local/bin` (make sure it is on the `PATH`), or `/usr/local/bin`.

`checksums.txt` in the same release has the SHA-256 of every archive.

The binaries are not code-signed. Windows SmartScreen may warn on first run; macOS quarantines files downloaded with
a browser (`xattr -d com.apple.quarantine dw` clears it).

If the releases page is empty, no release has been published yet: use `go install`.

## go install

Needs Go 1.26 or newer (`go version`):

```sh
go install github.com/rechedev9/docuware-cli/cmd/dw@latest
```

The binary goes to `$(go env GOPATH)/bin`: `%USERPROFILE%\go\bin` on Windows (the Go installer adds it to the
`PATH`), `~/go/bin` elsewhere.

## From source

```sh
git clone https://github.com/rechedev9/docuware-cli.git
cd docuware-cli
go build -o dw ./cmd/dw        # dw.exe on Windows
./dw --version
```

## Verify

```sh
dw --version
dw --help
```

Next: [Quickstart](quickstart.md).

## Update

Install the new version the same way, then refresh the Claude Code skill, whose text ships inside the binary:

```sh
dw skill install
```

Profiles, stored secrets and tokens are kept across updates.

## Uninstall

`dw logout` for each profile, then delete the binary, the skill folder (`~/.claude/skills/docuware`) and the folders in
[Configuration](configuration.md#files).
