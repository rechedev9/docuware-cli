---
title: Overview
description: "dw is a read-only Go CLI for DocuWare's Platform REST API, built to be driven by coding agents such as Claude Code."
---

# dw documentation

`dw` searches, reads and downloads documents in DocuWare (Cloud or on-premises, 7.10+) from the terminal. It is made
for coding agents first: every command has `--json`, exit codes are stable, errors list the valid choices, and output
is capped to keep context small. It cannot change or delete anything in DocuWare.

## Pick your path

- **An agent was asked to set this up:** [Agent setup runbook](agent-setup.md), start to finish.
- **Trying it yourself:** [Install](install.md) → [Quickstart](quickstart.md).
- **Preparing DocuWare** (admin): [DocuWare setup](docuware-setup.md).
- **Using it from Claude Code or another agent:** [Using dw from agents](agent-usage.md).
- **Writing searches:** [Search](search.md).
- **Scripting:** [Output and exit codes](output.md), [Commands](commands.md).
- **Sign-in, profiles, CI:** [Authentication](authentication.md), [Configuration](configuration.md).
- **Something fails:** [Troubleshooting](troubleshooting.md).

## For contributors

- [Architecture](architecture.md): goals, packages, request flow, security.
- [DocuWare API notes](docuware-api.md): endpoints, evidence, open questions.
- [Testing](testing.md): tests, the fake server, validating against a real DocuWare.
- [Releasing](releasing.md).
- [`AGENTS.md`](../AGENTS.md): rules for agents changing this repository.

## Project

MIT licensed. Not affiliated with or endorsed by DocuWare GmbH. The [changelog](../CHANGELOG.md) tracks releases.
Source: <https://github.com/rechedev9/docuware-cli>.
