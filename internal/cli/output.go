package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"golang.org/x/term"
)

func isTerminal(v any) bool {
	f, ok := v.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

// printJSON writes v as JSON: indented for humans, compact when piped.
func (a *app) printJSON(v any) error {
	enc := json.NewEncoder(a.stdout)
	enc.SetEscapeHTML(false)
	if isTerminal(a.stdout) {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(v)
}

func (a *app) table(headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(a.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	tw.Flush()
}

// cell flattens a value for a table cell and caps its width.
func cell(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max > 0 && utf8.RuneCountInString(s) > max {
		r := []rune(s)
		return string(r[:max-1]) + "…"
	}
	return s
}

func humanSize(n int64) string {
	switch {
	case n <= 0:
		return "-"
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

func fprintf(w io.Writer, format string, args ...any) { fmt.Fprintf(w, format, args...) }
