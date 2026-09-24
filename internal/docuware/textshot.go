package docuware

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TextFromTextshot flattens a DocuWare textshot (OCR result) into plain
// text: words joined by spaces, lines by newlines, and a marker between
// pages. Text and table zones are read in document order.
//
// DocuWare's JSON is converted from XML, so a list with one element may
// arrive as a bare object; asList handles both shapes.
func TextFromTextshot(raw []byte) (string, error) {
	var ts map[string]any
	if err := json.Unmarshal(raw, &ts); err != nil {
		return "", fmt.Errorf("unexpected textshot format: %w", err)
	}
	pages := asList(ts["Pages"])
	var out strings.Builder
	for i, p := range pages {
		page, _ := p.(map[string]any)
		if len(pages) > 1 {
			if i > 0 {
				out.WriteString("\n")
			}
			fmt.Fprintf(&out, "--- page %d ---\n", i+1)
		}
		for _, z := range asList(page["Items"]) {
			zone, _ := z.(map[string]any)
			switch zone["$type"] {
			case "TextZone":
				writeLines(&out, zone)
			case "TableZone":
				for _, c := range asList(zone["Cz"]) {
					cell, _ := c.(map[string]any)
					if inner, ok := cell["TextZone"].(map[string]any); ok {
						writeLines(&out, inner)
					}
				}
			}
		}
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

func writeLines(out *strings.Builder, zone map[string]any) {
	for _, l := range asList(zone["Ln"]) {
		line, _ := l.(map[string]any)
		var words []string
		for _, w := range asList(line["Items"]) {
			word, _ := w.(map[string]any)
			if word["$type"] != "Word" {
				continue
			}
			if v, _ := word["Value"].(string); v != "" {
				words = append(words, v)
			}
		}
		if len(words) > 0 {
			out.WriteString(strings.Join(words, " "))
			out.WriteString("\n")
		}
	}
}

func asList(v any) []any {
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		return x
	default:
		return []any{x}
	}
}
