---
title: Agent setup runbook
description: "Step-by-step instructions for an AI coding agent (Claude Code, Codex, ...) that has been asked to install and configure dw for a person."
---

# Agent setup runbook

You are an AI agent and someone asked you to set up `dw` so they can work with their DocuWare documents from
Claude Code (or another agent). Follow the steps in order. Every step ends with a check; do not continue past a
failed check, use [Troubleshooting](troubleshooting.md) instead.

Raw copy of this page for tools that fetch URLs:
<https://raw.githubusercontent.com/rechedev9/docuware-cli/main/docs/agent-setup.md>

## Ground rules

- **Never ask for, read, echo or store the DocuWare password or client secret.** The human types it into
  `dw login` in their own terminal. Do not put it in a command line, a file, an environment variable or the chat.
  If the human pastes a secret into the chat anyway, tell them to change it after setup.
- `dw` is read-only against DocuWare: no command changes or deletes documents. `dw download` writes files locally.
- Tell the human before you edit anything outside the `dw` install folder (the user `PATH`, shell profiles,
  `~/.claude/settings.json`).
- Your own shell keeps the `PATH` it started with. After installing, call `dw` by its full path until the human
  restarts the agent.

## 0. Ask the human (once, all together)

None of these are secrets:

1. **DocuWare address.** A Cloud tenant name (`acme` for `https://acme.docuware.cloud`) or the on-premises server
   URL. If they do not know it: it is the start of the address bar when they open DocuWare in the browser.
2. **DocuWare user name** they sign in with. If they sign in through Microsoft/Entra ID, ADFS or another single sign-on,
   the API cannot use that account (DocuWare does not allow SSO for the API); they need a DocuWare user with a
   DocuWare password, or an OAuth app with a client secret. See [DocuWare setup](docuware-setup.md).
3. **Scope of the Claude Code skill:** every project (default) or only the current project.
4. Whether you may add `dw` to Claude Code's allowed commands so it stops asking for permission on every call (step 7).

## 1. Detect the platform and an existing install

Windows (PowerShell):

```powershell
$env:PROCESSOR_ARCHITECTURE           # AMD64 or ARM64
Get-Command dw -ErrorAction SilentlyContinue | Select-Object Source
dw --version
```

macOS / Linux:

```sh
uname -sm                             # Darwin arm64, Linux x86_64, ...
command -v dw && dw --version
```

If `dw --version` already prints a version, skip to step 3 unless the human asked for an update.

## 2. Install the binary

Prefer **2a**: it needs nothing else. Use **2b** when there is no release yet or downloads are blocked.

### 2a. Prebuilt release

Check that a release exists: `https://api.github.com/repos/rechedev9/docuware-cli/releases/latest` must return
JSON with a `tag_name` (HTTP 404 means no release yet, go to 2b).

Windows (PowerShell 5.1 or 7). Installs to `%LOCALAPPDATA%\Programs\dw` and adds it to the user `PATH`:

```powershell
$ErrorActionPreference = 'Stop'; $ProgressPreference = 'SilentlyContinue'
$repo = 'rechedev9/docuware-cli'
$tag  = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
$ver  = $tag.TrimStart('v')
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$file = "dw_${ver}_windows_$arch.zip"
$zip  = Join-Path $env:TEMP $file
$sums = Join-Path $env:TEMP 'dw_checksums.txt'
Invoke-WebRequest "https://github.com/$repo/releases/download/$tag/$file" -OutFile $zip
Invoke-WebRequest "https://github.com/$repo/releases/download/$tag/checksums.txt" -OutFile $sums
$hash = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
if (-not (Select-String -Path $sums -SimpleMatch "$hash  $file" -Quiet)) { throw "checksum mismatch for $file" }
$dest = Join-Path $env:LOCALAPPDATA 'Programs\dw'
Expand-Archive $zip -DestinationPath $dest -Force
$userPath = [string][Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $dest) {
  [Environment]::SetEnvironmentVariable('Path', ($userPath.TrimEnd(';') + ";$dest").TrimStart(';'), 'User')
}
& (Join-Path $dest 'dw.exe') --version
```

macOS / Linux. Installs to `~/.local/bin`:

```sh
set -eu
repo=rechedev9/docuware-cli
tag=$(curl -fsSL "https://api.github.com/repos/$repo/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')
ver=${tag#v}
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m); case "$arch" in x86_64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; esac
file="dw_${ver}_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
curl -fsSL -o "$tmp/$file" "https://github.com/$repo/releases/download/$tag/$file"
curl -fsSL -o "$tmp/checksums.txt" "https://github.com/$repo/releases/download/$tag/checksums.txt"
cd "$tmp"
if command -v sha256sum >/dev/null; then grep "  $file\$" checksums.txt | sha256sum -c -
else grep "  $file\$" checksums.txt | shasum -a 256 -c -; fi
tar -xzf "$file" dw
mkdir -p "$HOME/.local/bin" && install -m 0755 dw "$HOME/.local/bin/dw"
"$HOME/.local/bin/dw" --version
```

If `~/.local/bin` is not on the `PATH` (`echo "$PATH"`), tell the human and add
`export PATH="$HOME/.local/bin:$PATH"` to `~/.zshrc` (macOS) or `~/.bashrc` (Linux).

### 2b. Build with Go

Needs Go 1.26 or newer (`go version`). If Go is missing or older, install it first:
`winget install --id GoLang.Go -e` (Windows), `brew install go` (macOS), or the archive from <https://go.dev/dl/> (Linux).

```sh
go install github.com/rechedev9/docuware-cli/cmd/dw@latest
```

The binary lands in `$(go env GOPATH)/bin` (`%USERPROFILE%\go\bin` on Windows, `~/go/bin` elsewhere). The Go
installer for Windows puts that folder on the user `PATH`; on macOS/Linux add it if `command -v dw` finds nothing.

**Check:** `dw --version` (or the full path) prints a version, and `dw --help` lists the commands.

## 3. The human signs in

Give the human the exact command with their values filled in and ask them to run it in a **new terminal window**
(Windows Terminal / PowerShell, macOS Terminal). Not in your shell, and not with Claude Code's `!` prefix: `dw login`
reads the password from a hidden prompt, which needs a real terminal.

```sh
dw login --url acme --user peggy.jenkins
```

- On-premises: `--url https://dms.example.com` (pass the scheme when the host name has no dots). Self-signed
  certificate: add `--insecure`, and only for a server they trust.
- OAuth app instead of a user: `dw login --url acme --client-id <client-id>` (prompts for the client secret).
- Success looks like `Logged in to https://acme.docuware.cloud (DocuWare 7.x) as peggy.jenkins, profile "default".`

Wait until they confirm.

**Check:** you run `dw status --json`. Exit code `0` and a JSON object with `"version"`, `"account"` and
`"file_cabinets"` mean the login works. Exit code `3` means it does not; read the `hint:` line on stderr and see
[Troubleshooting](troubleshooting.md#sign-in).

## 4. Install the Claude Code skill

The skill tells Claude when to reach for `dw` and how to search correctly. It is embedded in the binary, so it always
matches the installed version.

```sh
dw skill install                        # every project: ~/.claude/skills/docuware/SKILL.md
dw skill install --dir .claude/skills   # only the current project (run from the project root)
```

**Check:** the printed `SKILL.md` path exists.

Other agents: if they support Agent Skills, pass their skills directory to `--dir`. Otherwise append the output of
`dw skill` to the agent's instructions file (for example `AGENTS.md`). See [Using dw from agents](agent-usage.md).

## 5. Smoke test

```sh
dw cabinets --json
dw fields "<first cabinet name>" --json
dw search "<first cabinet name>" --limit 3 --json
```

**Check:** each exits `0`. `dw cabinets` lists at least one cabinet; an empty list means the DocuWare user has no
file cabinet rights (see [DocuWare setup](docuware-setup.md)).

## 6. Restart the agent

Ask the human to restart Claude Code (close and reopen it, or start a new session) so it loads the skill and the new
`PATH`. Then `/skills` must list `docuware`.

## 7. Optional: stop the permission prompts

With the human's consent, merge these entries into `permissions.allow` in `~/.claude/settings.json` (all projects) or
`.claude/settings.json` (one project). Keep every existing entry; create the file if it does not exist.

```json
{
  "permissions": {
    "allow": ["Bash(dw *)", "PowerShell(dw *)"]
  }
}
```

`Bash(...)` covers the Bash tool (macOS, Linux, Git Bash on Windows) and `PowerShell(...)` the PowerShell tool on
Windows. Because `dw` cannot change DocuWare data, allowing it is low risk. `dw download` still writes files into the
working directory.

## 8. Report back

Tell the human, briefly:

- where `dw` is installed and its version, the profile name and the DocuWare account it uses;
- how many file cabinets it can see;
- where the skill is (user-wide or project) and whether permissions were added;
- how to use it: just ask, e.g. *"Find the open invoices from Peters Engineering in DocuWare and summarize the
  biggest one"*, or call the skill with `/docuware`;
- how to update: repeat step 2, then `dw skill install` again.

## Uninstall

```sh
dw logout                     # removes the profile, its stored secret and cached tokens (repeat with -p <name>)
```

Then delete the skill folder (`~/.claude/skills/docuware` or `.claude/skills/docuware`), the binary, the `dw` entries
in `permissions.allow`, and the config and cache folders listed in [Configuration](configuration.md#files).
