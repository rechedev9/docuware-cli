package docuware

import (
	"fmt"
	"strconv"
	"strings"
)

// Query describes a search in CLI terms.
//
// Conditions use FIELD=VALUE, FIELD>=VALUE, FIELD<=VALUE or FIELD=FROM..TO.
// FIELD is the database name or the dialog label (case-insensitive).
// Text values keep DocuWare's wildcards (* and ?); EMPTY() and NOTEMPTY()
// match empty and non-empty fields. Repeating a text field ORs its values.
type Query struct {
	Conditions []string
	Or         bool
	// Sort entries are FIELD or FIELD:asc / FIELD:desc.
	Sort   []string
	Limit  int
	Offset int
}

// Expression is the body of a DialogExpression query.
type Expression struct {
	Condition []Condition `json:"Condition"`
	Operation string      `json:"Operation"`
	SortOrder []SortField `json:"SortOrder,omitempty"`
}

// Condition restricts one field. For number and date fields a two-element
// Value is a range, with null for an open end.
type Condition struct {
	DBName string    `json:"DBName"`
	Value  []*string `json:"Value"`
}

// SortField orders the result list.
type SortField struct {
	Field     string `json:"Field"`
	Direction string `json:"Direction"`
}

type fieldCond struct {
	field  DialogField
	values []string
	lo, hi *string
	ranged bool
}

// BuildExpression validates q against the dialog fields and builds the
// request body.
func BuildExpression(fields []DialogField, q Query) (Expression, error) {
	expr := Expression{Operation: "And", Condition: []Condition{}}
	if q.Or {
		expr.Operation = "Or"
	}
	var order []string
	conds := map[string]*fieldCond{}
	for _, raw := range q.Conditions {
		name, op, value, err := splitCondition(raw)
		if err != nil {
			return Expression{}, err
		}
		f, err := FindField(fields, name)
		if err != nil {
			return Expression{}, err
		}
		fc, ok := conds[f.DBFieldName]
		if !ok {
			fc = &fieldCond{field: f}
			conds[f.DBFieldName] = fc
			order = append(order, f.DBFieldName)
		}
		if err := fc.add(op, value); err != nil {
			return Expression{}, err
		}
	}
	for _, name := range order {
		fc := conds[name]
		c := Condition{DBName: name}
		if fc.ranged {
			c.Value = []*string{fc.lo, fc.hi}
		} else {
			for _, v := range fc.values {
				c.Value = append(c.Value, &v)
			}
		}
		expr.Condition = append(expr.Condition, c)
	}
	for _, s := range q.Sort {
		sf, err := parseSort(fields, s)
		if err != nil {
			return Expression{}, err
		}
		expr.SortOrder = append(expr.SortOrder, sf)
	}
	return expr, nil
}

// FindField resolves a field by database name or label, case-insensitively.
func FindField(fields []DialogField, name string) (DialogField, error) {
	for _, f := range fields {
		if strings.EqualFold(f.DBFieldName, name) {
			return f, nil
		}
	}
	for _, f := range fields {
		if strings.EqualFold(f.DlgLabel, name) {
			return f, nil
		}
	}
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.DBFieldName)
	}
	return DialogField{}, &NotFoundError{Kind: "search field", Key: name, Available: names}
}

func splitCondition(raw string) (name, op, value string, err error) {
	i := strings.IndexByte(raw, '=')
	if i <= 0 {
		return "", "", "", queryErr("invalid condition %q: use FIELD=VALUE, FIELD>=VALUE, FIELD<=VALUE or FIELD=FROM..TO", raw)
	}
	name, op, value = raw[:i], "=", strings.TrimSpace(raw[i+1:])
	switch {
	case strings.HasSuffix(name, ">"):
		name, op = name[:len(name)-1], ">="
	case strings.HasSuffix(name, "<"):
		name, op = name[:len(name)-1], "<="
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", "", queryErr("invalid condition %q: missing field name", raw)
	}
	return name, op, value, nil
}

func (fc *fieldCond) add(op, value string) error {
	f := fc.field
	kind := fieldKind(f.DWFieldType)
	if value == "" {
		return queryErr("empty value for %s; use %s=EMPTY() to find documents where it is empty", f.DBFieldName, f.DBFieldName)
	}
	if special, ok := specialValue(value); ok {
		if fc.ranged || len(fc.values) > 0 {
			return queryErr("%s: %s cannot be combined with other values", f.DBFieldName, special)
		}
		fc.values = append(fc.values, special)
		return nil
	}

	var lo, hi string
	isRange := false
	switch op {
	case ">=":
		lo, isRange = value, true
	case "<=":
		hi, isRange = value, true
	default:
		if kind != kindText {
			if a, b, ok := strings.Cut(value, ".."); ok {
				lo, hi, isRange = strings.TrimSpace(a), strings.TrimSpace(b), true
				if lo == "" && hi == "" {
					return queryErr("%s: empty range %q", f.DBFieldName, value)
				}
			}
		}
	}

	if isRange {
		if kind == kindText {
			return queryErr("%s is a text field; >=, <= and ranges only work on number and date fields (use wildcards like %s=ABC*)", f.DBFieldName, f.DBFieldName)
		}
		if len(fc.values) > 0 {
			return queryErr("%s: cannot mix a range with exact values", f.DBFieldName)
		}
		fc.ranged = true
		for _, p := range []struct {
			v   string
			dst **string
		}{{lo, &fc.lo}, {hi, &fc.hi}} {
			if p.v == "" {
				continue
			}
			if *p.dst != nil {
				return queryErr("%s: range bound given twice", f.DBFieldName)
			}
			v, err := normalizeValue(f, kind, p.v)
			if err != nil {
				return err
			}
			*p.dst = &v
		}
		return nil
	}

	if fc.ranged {
		return queryErr("%s: cannot mix a range with exact values", f.DBFieldName)
	}
	if kind != kindText && len(fc.values) > 0 {
		return queryErr("%s: DocuWare reads two values on a number or date field as a range; use %s=FROM..TO or run separate searches", f.DBFieldName, f.DBFieldName)
	}
	v, err := normalizeValue(f, kind, value)
	if err != nil {
		return err
	}
	fc.values = append(fc.values, v)
	return nil
}

type kind int

const (
	kindText kind = iota
	kindNumber
	kindDate
)

func fieldKind(dwType string) kind {
	switch strings.ToLower(dwType) {
	case "int", "numeric", "decimal":
		return kindNumber
	case "date", "datetime":
		return kindDate
	}
	return kindText
}

func specialValue(v string) (string, bool) {
	switch strings.ToUpper(strings.ReplaceAll(v, " ", "")) {
	case "EMPTY()":
		return "EMPTY()", true
	case "NOTEMPTY()":
		return "NOTEMPTY()", true
	}
	return "", false
}

func normalizeValue(f DialogField, k kind, v string) (string, error) {
	switch k {
	case kindNumber:
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			return "", queryErr("%s is a number field; %q is not a number (use a dot for decimals)", f.DBFieldName, v)
		}
		return v, nil
	case kindDate:
		if _, ok := ParseTime(v); !ok || strings.HasPrefix(v, "/Date(") {
			return "", queryErr("%s is a date field; %q is not a date (use YYYY-MM-DD)", f.DBFieldName, v)
		}
		return v, nil
	}
	return escapeParens(v), nil
}

// escapeParens escapes ( and ) so DocuWare does not read them as functions.
// Existing backslash escapes are kept, which makes the function idempotent.
func escapeParens(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			b.WriteByte(c)
			b.WriteByte(s[i+1])
			i++
			continue
		}
		if c == '(' || c == ')' {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String()
}

func parseSort(fields []DialogField, s string) (SortField, error) {
	name, dir, _ := strings.Cut(s, ":")
	name = strings.TrimSpace(name)
	direction := "Asc"
	switch strings.ToLower(strings.TrimSpace(dir)) {
	case "", "asc":
	case "desc":
		direction = "Desc"
	default:
		return SortField{}, queryErr("invalid sort %q: use FIELD, FIELD:asc or FIELD:desc", s)
	}
	if f, err := FindField(fields, name); err == nil {
		name = f.DBFieldName
	}
	// Unknown names pass through: system fields such as DWSTOREDATETIME are
	// sortable without being part of the search dialog.
	return SortField{Field: name, Direction: direction}, nil
}

// QueryError reports an invalid search condition or sort order.
type QueryError struct{ msg string }

func (e *QueryError) Error() string { return e.msg }

func queryErr(format string, args ...any) error {
	return &QueryError{msg: fmt.Sprintf(format, args...)}
}
