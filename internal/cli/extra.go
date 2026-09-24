package cli

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rechedev9/docuware-cli/internal/docuware"
	"github.com/rechedev9/docuware-cli/skill"
)

func (a *app) apiCmd() *cobra.Command {
	var params []string
	cmd := &cobra.Command{
		Use:   "api <path>",
		Short: "GET any Platform REST path and print the raw JSON",
		Long: `Escape hatch for resources dw has no command for yet (workflows,
stamps, users, ...). The path is relative to /DocuWare/Platform unless it
starts with /DocuWare/. Links found in responses can be passed back as-is.
Only GET is supported, so this command cannot change data.`,
		Example: `  dw api Organizations
  dw api FileCabinets/<cabinet-id>
  dw api FileCabinets/<cabinet-id>/Query/Documents -q count=5`,
		Args: exactArgs(1, "a Platform path"),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			for _, p := range params {
				k, v, ok := strings.Cut(p, "=")
				if !ok || k == "" {
					return usageErr("invalid query parameter %q: use key=value", p)
				}
				q.Add(k, v)
			}
			ctx := cmd.Context()
			c, err := a.connect(ctx)
			if err != nil {
				return err
			}
			b, err := c.Get(ctx, platformPath(args[0]), q)
			if err != nil {
				return err
			}
			if isTerminal(a.stdout) {
				var buf bytes.Buffer
				if json.Indent(&buf, b, "", "  ") == nil {
					b = buf.Bytes()
				}
			}
			b = bytes.TrimRight(b, "\r\n")
			_, err = a.stdout.Write(append(b, '\n'))
			return err
		},
	}
	cmd.Flags().StringArrayVarP(&params, "query", "q", nil, "query parameter key=value (repeatable)")
	return cmd
}

func platformPath(p string) string {
	lower := strings.ToLower(p)
	switch {
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"),
		strings.HasPrefix(lower, "/docuware/"):
		return p
	case strings.HasPrefix(p, "/"):
		return docuware.PlatformPath + p
	default:
		return docuware.PlatformPath + "/" + p
	}
}

func (a *app) skillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Print the Claude Code skill that teaches agents to use dw",
		Args:  exactArgs(0, "no arguments"),
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := a.stdout.Write([]byte(skill.Markdown))
			return err
		},
	}
	var dir string
	install := &cobra.Command{
		Use:   "install",
		Short: "Install the skill for Claude Code (~/.claude/skills/docuware)",
		Args:  exactArgs(0, "no arguments"),
		RunE: func(_ *cobra.Command, _ []string) error {
			if dir == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				dir = filepath.Join(home, ".claude", "skills")
			}
			target := filepath.Join(dir, "docuware", "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(target, []byte(skill.Markdown), 0o644); err != nil {
				return err
			}
			if a.jsonOut {
				return a.printJSON(map[string]string{"path": target})
			}
			fprintf(a.stdout, "Installed %s\n\n", target)
			fprintf(a.stdout, "Next steps:\n")
			fprintf(a.stdout, "  1. Run `dw login --url <tenant> --user <name>` in a normal terminal (Claude Code cannot answer password prompts).\n")
			fprintf(a.stdout, "  2. Restart Claude Code and check /skills lists \"docuware\".\n")
			fprintf(a.stdout, "  3. Optional, to skip permission prompts (dw is read-only): add \"Bash(dw *)\" and \"PowerShell(dw *)\"\n")
			fprintf(a.stdout, "     to permissions.allow in ~/.claude/settings.json.\n")
			return nil
		},
	}
	install.Flags().StringVar(&dir, "dir", "", "skills directory (default: ~/.claude/skills; use .claude/skills for one project)")
	cmd.AddCommand(install)
	return cmd
}
