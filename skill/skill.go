// Package skill embeds the Claude Code skill shipped with dw.
package skill

import _ "embed"

// Markdown is the content of SKILL.md.
//
//go:embed SKILL.md
var Markdown string
