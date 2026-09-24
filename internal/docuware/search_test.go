package docuware

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

var testFields = []DialogField{
	{DBFieldName: "COMPANY", DlgLabel: "Company", DWFieldType: "Text"},
	{DBFieldName: "INVOICE_DATE", DlgLabel: "Invoice date", DWFieldType: "Date"},
	{DBFieldName: "AMOUNT", DlgLabel: "Amount", DWFieldType: "Decimal"},
	{DBFieldName: "INVOICE_NO", DWFieldType: "Int"},
}

func TestBuildExpression(t *testing.T) {
	cases := []struct {
		name  string
		q     Query
		want  string
		error string
	}{
		{
			name: "exact and wildcard",
			q:    Query{Conditions: []string{"COMPANY=Peters*"}},
			want: `{"Condition":[{"DBName":"COMPANY","Value":["Peters*"]}],"Operation":"And"}`,
		},
		{
			name: "label, repeated field with OR, parentheses escaped",
			q:    Query{Conditions: []string{"company=Acme (EU)", "Company=Beta", "COMPANY=EMPTY()"}, Or: true},
			want: `{"Condition":[{"DBName":"COMPANY","Value":["Acme \\(EU\\)"]},{"DBName":"COMPANY","Value":["Beta"]},{"DBName":"COMPANY","Value":["EMPTY()"]}],"Operation":"Or"}`,
		},
		{
			name: "two exact dates with OR",
			q:    Query{Conditions: []string{"INVOICE_DATE=2024-01-01", "INVOICE_DATE=2024-02-01"}, Or: true},
			want: `{"Condition":[{"DBName":"INVOICE_DATE","Value":["2024-01-01"]},{"DBName":"INVOICE_DATE","Value":["2024-02-01"]}],"Operation":"Or"}`,
		},
		{
			name: "date range and open bounds merged",
			q:    Query{Conditions: []string{"INVOICE_DATE=2024-01-01..2024-06-30", "AMOUNT>=100", "AMOUNT<=250.5", "INVOICE_NO=..10"}},
			want: `{"Condition":[{"DBName":"INVOICE_DATE","Value":["2024-01-01","2024-06-30"]},{"DBName":"AMOUNT","Value":["100","250.5"]},{"DBName":"INVOICE_NO","Value":[null,"10"]}],"Operation":"And"}`,
		},
		{
			name: "empty checks and sort",
			q:    Query{Conditions: []string{"COMPANY=empty()"}, Sort: []string{"amount:desc", "DWSTOREDATETIME"}},
			want: `{"Condition":[{"DBName":"COMPANY","Value":["EMPTY()"]}],"Operation":"And","SortOrder":[{"Field":"AMOUNT","Direction":"Desc"},{"Field":"DWSTOREDATETIME","Direction":"Asc"}]}`,
		},
		{
			name: "dots in text are literal",
			q:    Query{Conditions: []string{"COMPANY=A..B"}},
			want: `{"Condition":[{"DBName":"COMPANY","Value":["A..B"]}],"Operation":"And"}`,
		},
		{name: "unknown field", q: Query{Conditions: []string{"NOPE=1"}}, error: `search field "NOPE" not found; available: COMPANY`},
		{name: "missing operator", q: Query{Conditions: []string{"COMPANY"}}, error: "use FIELD=VALUE"},
		{name: "empty value", q: Query{Conditions: []string{"COMPANY="}}, error: "COMPANY=EMPTY()"},
		{name: "range on text", q: Query{Conditions: []string{"COMPANY>=A"}}, error: "text field"},
		{name: "repeated text without OR", q: Query{Conditions: []string{"COMPANY=A", "COMPANY=B"}}, error: "add --or"},
		{name: "two exact dates", q: Query{Conditions: []string{"INVOICE_DATE=2024-01-01", "INVOICE_DATE=2024-02-01"}}, error: "INVOICE_DATE=FROM..TO"},
		{name: "empty with range", q: Query{Conditions: []string{"AMOUNT>=1", "AMOUNT=EMPTY()"}}, error: "cannot be combined"},
		{name: "bad date", q: Query{Conditions: []string{"INVOICE_DATE=15.01.2024"}}, error: "YYYY-MM-DD"},
		{name: "bad number", q: Query{Conditions: []string{"AMOUNT=1,5"}}, error: "not a number"},
		{name: "bound twice", q: Query{Conditions: []string{"AMOUNT>=1", "AMOUNT>=2"}}, error: "given twice"},
		{name: "bad sort", q: Query{Sort: []string{"AMOUNT:up"}}, error: "invalid sort"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := BuildExpression(testFields, tc.q)
			if tc.error != "" {
				if err == nil || !strings.Contains(err.Error(), tc.error) {
					t.Fatalf("error = %v, want it to contain %q", err, tc.error)
				}
				var qe *QueryError
				var nf *NotFoundError
				if !errors.As(err, &qe) && !errors.As(err, &nf) {
					t.Errorf("error type %T, want QueryError or NotFoundError", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, _ := json.Marshal(expr)
			if string(got) != tc.want {
				t.Errorf("\n got %s\nwant %s", got, tc.want)
			}
		})
	}
}

func TestFieldValueDecoding(t *testing.T) {
	jan15 := time.Date(2024, 1, 15, 0, 0, 0, 0, time.Local).UnixMilli()
	cases := []struct {
		json string
		want any
	}{
		{`{"ItemElementName":"String","Item":"ACME"}`, "ACME"},
		{`{"ItemElementName":"String","Item":""}`, nil},
		{`{"ItemElementName":"String","Item":null}`, nil},
		{`{"ItemElementName":"Int","Item":42}`, int64(42)},
		{`{"ItemElementName":"Decimal","Item":12.5}`, 12.5},
		{`{"ItemElementName":"Date","Item":"/Date(` + itoa(jan15) + `)/"}`, "2024-01-15"},
		{`{"ItemElementName":"DateTime","Item":"/Date(1705312800000+0100)/"}`, "2024-01-15T11:00:00+01:00"},
		{`{"ItemElementName":"Date","Item":"/Date(-62135596800000)/"}`, nil},
		{`{"ItemElementName":"Keywords","Item":{"Keyword":["a","b"]}}`, "a, b"},
		{`{"ItemElementName":"Memo","Item":"long text"}`, "long text"},
		{`{"ItemElementName":"Keywords","Item":{"$type":"DocumentIndexFieldKeywords","Keyword":[]}}`, nil},
		{`{"ItemElementName":"Table","Item":{"Row":[{"ColumnValue":[{"FieldName":"POS","ItemElementName":"Int","Item":1}]}]}}`, `[{"POS":1}]`},
		{`{"ItemElementName":"String","Item":"x","IsNull":true}`, nil},
	}
	for _, tc := range cases {
		var f FieldValue
		if err := json.Unmarshal([]byte(tc.json), &f); err != nil {
			t.Fatal(err)
		}
		got := f.Value()
		switch got.(type) {
		case []string, []map[string]any:
			got = FormatValue(got)
		}
		if got != tc.want {
			t.Errorf("%s: got %#v, want %#v", tc.json, got, tc.want)
		}
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestCountAcceptsBothShapes(t *testing.T) {
	var a, b QueryResult
	_ = json.Unmarshal([]byte(`{"Count":{"Value":7,"HasMore":true}}`), &a)
	_ = json.Unmarshal([]byte(`{"Count":5}`), &b)
	if a.Count.Value != 7 || !a.Count.HasMore || b.Count.Value != 5 {
		t.Errorf("got %+v and %+v", a.Count, b.Count)
	}
}

func TestSanitizeFileName(t *testing.T) {
	cases := map[string]string{
		"invoice.pdf":         "invoice.pdf",
		`..\..\evil.pdf`:      "evil.pdf",
		"../../etc/passwd":    "passwd",
		"C:evil.pdf":          "evil.pdf",
		`a<b>c|d?.pdf`:        "a_b_c_d_.pdf",
		"..":                  "fallback",
		"":                    "fallback",
		"trailing dots... ":   "trailing dots",
		"Rechnung Müller.pdf": "Rechnung Müller.pdf",
	}
	for in, want := range cases {
		if got := SanitizeFileName(in, "fallback"); got != want {
			t.Errorf("SanitizeFileName(%q) = %q, want %q", in, got, want)
		}
	}
}
