---
title: Using dw from agents
description: "How coding agents such as Claude Code should call dw: the skill, permissions, JSON output, paging, exit codes and keeping context small."
---

# Using dw from agents

`dw` was designed to be driven by an agent in a terminal: one command per question, `--json` on everything, stable
exit codes, errors that name the valid choices, and output capped so a single call cannot flood the context window.
Setting it up the first time is covered in the [Agent setup runbook](agent-setup.md).

## Claude Code

### The skill

`dw skill install` writes `SKILL.md` to `~/.claude/skills/docuware/` (or `--dir <skills dir>`). Claude Code reads the
skill's description at startup and loads the rest when a request is about DocuWare, so the user can simply ask. They
can also invoke it explicitly with `/docuware`.

- The skill text is embedded in the binary. After upgrading `dw`, run `dw skill install` again.
- `dw skill` prints it without installing, for review or for agents without skill support.
- User-wide (`~/.claude/skills`) and project (`.claude/skills` in the repo) installs both work; a project install
  can be committed so a team gets it with the repository.

### Permissions

Each `dw` call goes through Claude Code's permission prompt unless allowed. `dw` cannot modify DocuWare, so allowing
it is reasonable:

```json
{
  "permissions": {
    "allow": ["Bash(dw *)", "PowerShell(dw *)"]
  }
}
```

Put it in `~/.claude/settings.json` (all projects) or `.claude/settings.json` (one project), merged with existing
entries.

### No MCP server

`dw` is a plain CLI. There is nothing to add to Claude Code's MCP configuration.

### Signing in

Claude Code's shell has no terminal for the hidden password prompt, so `dw login` has to run in a normal terminal
(the `!` prefix does not help either). Once the human has signed in, tokens renew automatically and Claude never needs
the password. If a command exits with code `3`, ask the human to run `dw login` again; never ask for the password.

## Other agents

- Agents that read Agent Skills (`SKILL.md` folders): `dw skill install --dir <their skills directory>`.
- Everything else: append `dw skill` to the instructions file the agent reads (`AGENTS.md`, a rules file, a system
  prompt).
- Headless or CI agents can skip `dw login` and pass credentials through the environment; see
  [Authentication](authentication.md#environment-variables).

## Calling patterns

The workflow the skill teaches:

```sh
dw status --json                                   # logged in? which server and account?
dw cabinets --json                                 # names and ids
dw fields Invoices --json                          # field names, labels, types, value lists
dw values Invoices STATUS --json                   # allowed values of a list field
dw search Invoices COMPANY=Peters* "AMOUNT>=1000" --json
dw get Invoices 42 --json                          # all index fields and the files
dw text Invoices 42                                # OCR text, capped at 20000 characters
dw download Invoices 42 -o ./out/ --json           # prints the saved path
```

- **Look before searching.** Run `dw fields <cabinet>` before the first search in a cabinet. Field names come from
  the cabinet's search dialog and are not guessable.
- **Put every restriction in the conditions.** A search with `--limit 20` and no conditions only shows the first 20
  stored documents. Filtering them afterwards gives wrong answers.
- **Quote conditions with `<`, `>` or spaces** so the shell does not interpret them: `"AMOUNT>=1000"`,
  `"COMPANY=Peters Engineering"`.
- **One value per field unless `--or`.** `STATUS=Open STATUS=Overdue` is refused; add `--or` (which ORs all
  conditions) or run one search per value. See [Search](search.md).
- **Page deliberately.** JSON results carry `total`, `has_more` and `next_offset`. Fetch the next page with
  `--offset <next_offset>` only when the task needs it; raise `--limit` for counts instead of paging everything.
- **Keep text small.** `dw text` stops at 20000 characters and says so on stderr. Use `--max-chars` to read less, or
  `--section <id>` for one file of a multi-file document.
- **Downloads never overwrite.** An existing file gets a numbered name (`invoice (1).pdf`); the JSON output has the
  real `path`.

## Reading results

- stdout carries the result, stderr carries notes, warnings and errors. With `--json`, stdout is always one JSON
  document (indented in a terminal, compact when piped).
- Exit codes: `0` ok, `1` other error, `2` usage or invalid condition, `3` authentication, `4` not found.
- Errors have an `error:` line and often a `hint:` line. "Not found" errors list the valid names:

  ```text
  error: search field "NOPE" not found; available: COMPANY, INVOICE_DATE, AMOUNT, INVOICE_NO, STATUS
  ```

  Read the list and retry with a real name instead of guessing again.

Every JSON shape is documented in [Output](output.md).

## Anything without a command

`dw api <path>` performs a GET on any Platform REST path and prints the raw JSON. Paths are relative to
`/DocuWare/Platform`; `href` values from a response's `Links` can be passed back unchanged:

```sh
dw api Organizations
dw api FileCabinets/<cabinet-id>/Query/Documents -q count=5
```

Only GET is supported, so this stays read-only too.
