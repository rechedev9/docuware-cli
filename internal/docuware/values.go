package docuware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var dateRe = regexp.MustCompile(`^/Date\((-?\d+)([+-]\d{4})?\)/$`)

// dotnetMinDate is DateTime.MinValue, which DocuWare uses for "no date".
const dotnetMinDate = -62135596800000

// ParseTime parses DocuWare's "/Date(ms)/" format, falling back to ISO 8601.
func ParseTime(s string) (time.Time, bool) {
	if m := dateRe.FindStringSubmatch(s); m != nil {
		ms, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || ms == dotnetMinDate {
			return time.Time{}, false
		}
		t := time.UnixMilli(ms)
		if off := m[2]; off != "" {
			h, _ := strconv.Atoi(off[1:3])
			mi, _ := strconv.Atoi(off[3:5])
			secs := h*3600 + mi*60
			if off[0] == '-' {
				secs = -secs
			}
			t = t.In(time.FixedZone("", secs))
		}
		return t, true
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// Value decodes the field into a plain Go value: string, int64, float64,
// a date string (YYYY-MM-DD), an RFC 3339 timestamp, []string for keywords,
// []map[string]any for table fields, or nil when empty.
func (f FieldValue) Value() any {
	raw := bytes.TrimSpace(f.Item)
	if f.IsNull || len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	switch strings.ToLower(f.ItemElementName) {
	case "string", "memo":
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if s == "" {
				return nil
			}
			return s
		}
	case "int":
		var n json.Number
		if json.Unmarshal(raw, &n) == nil {
			if i, err := n.Int64(); err == nil {
				return i
			}
		}
	case "decimal":
		var n json.Number
		if json.Unmarshal(raw, &n) == nil {
			if v, err := n.Float64(); err == nil {
				return v
			}
		}
	case "date", "datetime":
		var s string
		if json.Unmarshal(raw, &s) == nil {
			t, ok := ParseTime(s)
			if !ok {
				return nil
			}
			if strings.EqualFold(f.ItemElementName, "date") {
				return t.Format("2006-01-02")
			}
			return t.Format(time.RFC3339)
		}
	case "keywords", "keyword":
		var k struct {
			Keyword []string `json:"Keyword"`
		}
		if json.Unmarshal(raw, &k) == nil {
			if len(k.Keyword) == 0 {
				return nil
			}
			return k.Keyword
		}
	case "table":
		var t struct {
			Row []struct {
				ColumnValue []FieldValue `json:"ColumnValue"`
			} `json:"Row"`
		}
		if json.Unmarshal(raw, &t) == nil {
			rows := make([]map[string]any, 0, len(t.Row))
			for _, r := range t.Row {
				row := map[string]any{}
				for _, col := range r.ColumnValue {
					row[col.FieldName] = col.Value()
				}
				rows = append(rows, row)
			}
			if len(rows) == 0 {
				return nil
			}
			return rows
		}
	}
	var v any
	if json.Unmarshal(raw, &v) == nil {
		return v
	}
	return string(raw)
}

// FormatValue renders a decoded field value for terminal output.
func FormatValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []string:
		return strings.Join(x, ", ")
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	case []map[string]any:
		b, _ := json.Marshal(x)
		return string(b)
	default:
		return fmt.Sprint(x)
	}
}
