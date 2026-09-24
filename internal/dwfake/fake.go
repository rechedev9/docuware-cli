// Package dwfake is an in-memory DocuWare Platform server for tests.
//
// Response shapes follow the official REST samples and the JSON the
// reference clients parse: PascalCase properties, "Links" arrays,
// "/Date(ms)/" timestamps and XML-style single-element lists in textshots.
package dwfake

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const platform = "/DocuWare/Platform"

// Server is a fake DocuWare server.
type Server struct {
	*httptest.Server

	Username, Password     string
	ClientID, ClientSecret string
	// PageSize caps the items per result page, to exercise "next" links.
	PageSize int

	mu             sync.Mutex
	tokens         map[string]bool
	tokenRequests  int
	throttle       int
	lastExpression []byte
	lastQuery      map[string][]string
	requests       []string
	results        map[string][]doc
}

// New starts a fake server; call Close when done.
func New() *Server {
	s := &Server{
		Username: "peggy", Password: "s3cret",
		ClientID: "svc-app", ClientSecret: "svc-secret",
		PageSize: 2,
		tokens:   map[string]bool{},
		results:  map[string][]doc{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+platform+"/Home/IdentityServiceInfo", s.identityInfo)
	mux.HandleFunc("GET /identity/.well-known/openid-configuration", s.openID)
	mux.HandleFunc("POST /identity/connect/token", s.token)

	api := http.NewServeMux()
	api.HandleFunc("GET "+platform, s.root)
	api.HandleFunc("GET "+platform+"/Organizations", s.organizations)
	api.HandleFunc("GET "+platform+"/FileCabinets", s.cabinets)
	api.HandleFunc("GET "+platform+"/FileCabinets/{fc}/Dialogs", s.dialogs)
	api.HandleFunc("GET "+platform+"/FileCabinets/{fc}/Dialogs/{dlg}", s.dialog)
	api.HandleFunc("GET "+platform+"/FileCabinets/{fc}/SelectList/{field}", s.selectList)
	api.HandleFunc("POST "+platform+"/FileCabinets/{fc}/Query/DialogExpression", s.dialogExpression)
	api.HandleFunc("GET "+platform+"/FileCabinets/{fc}/Query/Documents", s.listDocuments)
	api.HandleFunc("GET "+platform+"/FileCabinets/{fc}/Query/Results/{qid}", s.resultPage)
	api.HandleFunc("GET "+platform+"/FileCabinets/{fc}/Documents/{id}", s.document)
	api.HandleFunc("GET "+platform+"/FileCabinets/{fc}/Documents/{id}/FileDownload", s.download)
	api.HandleFunc("GET "+platform+"/FileCabinets/{fc}/Sections/{sid}", s.section)
	api.HandleFunc("GET "+platform+"/FileCabinets/{fc}/Sections/{sid}/Textshot", s.textshot)
	api.HandleFunc("GET "+platform+"/FileCabinets/{fc}/Sections/{sid}/Data", s.sectionData)
	mux.Handle(platform+"/", s.authorized(api))
	mux.Handle(platform, s.authorized(api))

	s.Server = httptest.NewServer(mux)
	return s
}

// ExpireTokens invalidates all issued tokens, as if they timed out.
func (s *Server) ExpireTokens() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens = map[string]bool{}
}

// Throttle makes the next n API requests fail with 429.
func (s *Server) Throttle(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.throttle = n
}

// TokenRequests counts issued tokens.
func (s *Server) TokenRequests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokenRequests
}

// LastExpression returns the body of the last DialogExpression request.
func (s *Server) LastExpression() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastExpression
}

// LastQuery returns the query string of the last search or download.
func (s *Server) LastQuery() map[string][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastQuery
}

// Requests lists "METHOD path" of every authorized API request.
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func notFound(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusNotFound, map[string]any{"Message": msg, "Status": 404})
}

func (s *Server) base(r *http.Request) string { return "http://" + r.Host }

func (s *Server) authorized(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		s.mu.Lock()
		ok := s.tokens[tok]
		throttled := ok && s.throttle > 0
		if throttled {
			s.throttle--
		}
		if ok && !throttled {
			s.requests = append(s.requests, r.Method+" "+r.URL.Path)
		}
		s.mu.Unlock()
		switch {
		case !ok:
			writeJSON(w, http.StatusUnauthorized, map[string]any{"Message": "Unauthorized", "Status": 401})
		case throttled:
			w.Header().Set("Retry-After", "0")
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"Message": "Too many requests", "Status": 429})
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func (s *Server) identityInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"IdentityServiceUrl": s.base(r) + "/identity"})
}

func (s *Server) openID(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"issuer":                 s.base(r) + "/identity",
		"authorization_endpoint": s.base(r) + "/identity/connect/authorize",
		"token_endpoint":         s.base(r) + "/identity/connect/token",
	})
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid_request"})
		return
	}
	f := r.PostForm
	var ok bool
	switch f.Get("grant_type") {
	case "password":
		ok = f.Get("client_id") == "docuware.platform.net.client" && f.Get("scope") == "docuware.platform" &&
			f.Get("username") == s.Username && f.Get("password") == s.Password
	case "client_credentials":
		ok = f.Get("client_id") == s.ClientID && f.Get("client_secret") == s.ClientSecret
	}
	if !ok {
		writeJSON(w, 400, map[string]any{"error": "invalid_grant", "error_description": "invalid_username_or_password"})
		return
	}
	s.mu.Lock()
	s.tokenRequests++
	tok := fmt.Sprintf("tok-%d", s.tokenRequests)
	s.tokens[tok] = true
	s.mu.Unlock()
	writeJSON(w, 200, map[string]any{"access_token": tok, "expires_in": 3600, "token_type": "Bearer", "scope": "docuware.platform"})
}

func (s *Server) root(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{
		"Version": "7.12.0.0",
		"Links":   []link{{"organizations", platform + "/Organizations"}, {"filecabinets", platform + "/FileCabinets"}},
	})
}

func (s *Server) organizations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"Organization": []map[string]any{{"Name": "Peters Engineering", "Id": "org-1"}}})
}

type link struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}

const invoices = "fc-inv"

func fcPath(id string) string { return platform + "/FileCabinets/" + id }

func (s *Server) cabinets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"FileCabinet": []map[string]any{
		{"Id": invoices, "Name": "Invoices", "IsBasket": false, "Default": true, "Color": "Yellow",
			"Links": []link{{"dialogs", fcPath(invoices) + "/Dialogs"}, {"documents", fcPath(invoices) + "/Documents"}}},
		{"Id": "fc-contracts", "Name": "Contracts", "IsBasket": false},
		{"Id": "b-inbox", "Name": "Inbox", "IsBasket": true},
	}})
}

func (s *Server) dialogs(w http.ResponseWriter, r *http.Request) {
	fc := r.PathValue("fc")
	if fc != invoices {
		writeJSON(w, 200, map[string]any{"Dialog": []any{}})
		return
	}
	info := func(id, name, typ string, def bool) map[string]any {
		return map[string]any{"$type": "DialogInfo", "Id": id, "DisplayName": name, "Type": typ, "IsDefault": def,
			"FileCabinetId": fc, "Links": []link{{"self", fcPath(fc) + "/Dialogs/" + id}}}
	}
	writeJSON(w, 200, map[string]any{"Dialog": []any{
		info("dlg-store", "Store invoice", "Store", false),
		info("dlg-search", "Invoice search", "Search", true),
		info("dlg-list", "Invoice list", "ResultList", false),
		info("dlg_internal", "Internal", "Search", false),
	}})
}

type fieldDef struct {
	name, label, typ string
	list             bool
}

var searchFields = []fieldDef{
	{"COMPANY", "Company", "Text", false},
	{"INVOICE_DATE", "Invoice date", "Date", false},
	{"AMOUNT", "Amount", "Decimal", false},
	{"INVOICE_NO", "Invoice no.", "Numeric", false},
	{"STATUS", "Status", "Text", true},
}

func (s *Server) dialog(w http.ResponseWriter, r *http.Request) {
	fc, id := r.PathValue("fc"), r.PathValue("dlg")
	if fc != invoices || id != "dlg-search" {
		notFound(w, "Dialog not found")
		return
	}
	var fields []map[string]any
	for _, f := range searchFields {
		m := map[string]any{"DBFieldName": f.name, "DlgLabel": f.label, "DWFieldType": f.typ, "Length": 64, "Visible": true}
		if f.list {
			m["Links"] = []link{{"simpleSelectList", fcPath(fc) + "/SelectList/" + f.name}}
		}
		fields = append(fields, m)
	}
	writeJSON(w, 200, map[string]any{
		"Id": id, "DisplayName": "Invoice search", "Type": "Search", "FileCabinetId": fc, "IsDefault": true,
		"Fields": fields,
		"Query":  map[string]any{"Links": []link{{"dialogExpression", fcPath(fc) + "/Query/DialogExpression?dialogId=" + id}}},
		"Links":  []link{{"self", fcPath(fc) + "/Dialogs/" + id}},
	})
}

func (s *Server) selectList(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("field") != "STATUS" {
		notFound(w, "no list")
		return
	}
	writeJSON(w, 200, map[string]any{"Value": []string{"Open", "Paid", "Overdue"}})
}

type doc struct {
	id       int64
	company  string
	date     time.Time
	amount   float64
	number   int64
	status   string
	tags     []string
	file     string
	textshot bool
}

var docs = []doc{
	{1, "Peters Engineering", day(2024, 1, 15), 1250.5, 1001, "Paid", []string{"urgent", "q1"}, "invoice-1001.pdf", true},
	{2, "Acme Corp (EU)", day(2024, 3, 2), 99.9, 1002, "Open", nil, "invoice-1002.pdf", false},
	{3, "Peters Consulting", day(2024, 6, 30), 5000, 1003, "Open", nil, `..\..\evil.pdf`, true},
}

func day(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.Local) }

func dwDate(t time.Time) string { return fmt.Sprintf("/Date(%d)/", t.UnixMilli()) }

func (d doc) fields() []map[string]any {
	f := func(name, label, typ string, item any, system bool) map[string]any {
		return map[string]any{"FieldName": name, "FieldLabel": label, "ItemElementName": typ, "Item": item,
			"ReadOnly": system, "SystemField": system}
	}
	var tags any
	if d.tags != nil {
		tags = map[string]any{"$type": "DocumentIndexFieldKeywords", "Keyword": d.tags}
	}
	return []map[string]any{
		f("COMPANY", "Company", "String", d.company, false),
		f("INVOICE_DATE", "Invoice date", "Date", dwDate(d.date), false),
		f("AMOUNT", "Amount", "Decimal", d.amount, false),
		f("INVOICE_NO", "Invoice no.", "Int", d.number, false),
		f("STATUS", "Status", "String", d.status, false),
		f("TAGS", "Tags", "Keywords", tags, false),
		f("CONTACT", "Contact", "String", nil, false),
		f("DWDOCID", "Document ID", "Int", d.id, true),
		f("DWSTOREDATETIME", "Stored on", "DateTime", dwDate(d.date.Add(9*time.Hour)), true),
	}
}

func (d doc) links() []link {
	self := fcPath(invoices) + "/Documents/" + strconv.FormatInt(d.id, 10)
	return []link{{"self", self}, {"fileDownload", self + "/FileDownload"}, {"fields", self + "/Fields"}}
}

func (d doc) summary() map[string]any {
	return map[string]any{"Id": d.id, "Title": d.company, "ContentType": "application/pdf",
		"FileCabinetId": invoices, "Fields": d.fields(), "Links": d.links()}
}

func sectionID(d doc) string { return fmt.Sprintf("%d-sec", d.id) }

func findDoc(id string) (doc, bool) {
	for _, d := range docs {
		if strconv.FormatInt(d.id, 10) == id {
			return d, true
		}
	}
	return doc{}, false
}

func (s *Server) document(w http.ResponseWriter, r *http.Request) {
	d, ok := findDoc(r.PathValue("id"))
	if r.PathValue("fc") != invoices || !ok {
		notFound(w, "Document not found")
		return
	}
	m := d.summary()
	m["FileSize"] = 2048
	m["CreatedAt"] = dwDate(d.date.Add(9 * time.Hour))
	m["LastModified"] = dwDate(d.date.Add(10 * time.Hour))
	sid := sectionID(d)
	// Embedded sections lack the textshot link on purpose: clients must load the section.
	m["Sections"] = []map[string]any{{"Id": sid, "ContentType": "application/pdf", "OriginalFileName": d.file,
		"FileSize": 2048, "PageCount": 2, "Links": []link{{"self", fcPath(invoices) + "/Sections/" + sid}}}}
	writeJSON(w, 200, m)
}

func (s *Server) section(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	for _, d := range docs {
		if sectionID(d) != sid {
			continue
		}
		links := []link{{"self", fcPath(invoices) + "/Sections/" + sid}, {"fileDownload", fcPath(invoices) + "/Sections/" + sid + "/Data"}}
		if d.textshot {
			links = append(links, link{"textshot", fcPath(invoices) + "/Sections/" + sid + "/Textshot"})
		}
		writeJSON(w, 200, map[string]any{"Id": sid, "ContentType": "application/pdf", "OriginalFileName": d.file, "Links": links})
		return
	}
	notFound(w, "Section not found")
}

func (s *Server) textshot(w http.ResponseWriter, r *http.Request) {
	word := func(v string) map[string]any { return map[string]any{"$type": "Word", "Value": v} }
	writeJSON(w, 200, map[string]any{"Pages": []any{
		map[string]any{"Items": []any{
			map[string]any{"$type": "TextZone", "Ln": []any{
				map[string]any{"Items": []any{word("INVOICE"), word("1001")}},
				map[string]any{"Items": word("Total")}, // single word as bare object (XML-style)
			}},
			map[string]any{"$type": "TableZone", "Cz": []any{
				map[string]any{"TextZone": map[string]any{"Ln": map[string]any{"Items": []any{word("1250.50"), word("EUR")}}}},
			}},
		}},
		map[string]any{"Items": map[string]any{"$type": "TextZone", "Ln": map[string]any{"Items": []any{word("Page"), word("two")}}}},
	}})
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	d, ok := findDoc(r.PathValue("id"))
	if !ok {
		notFound(w, "Document not found")
		return
	}
	s.mu.Lock()
	s.lastQuery = r.URL.Query()
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", d.file))
	fmt.Fprintf(w, "%%PDF-1.4 fake document %d", d.id)
}

type expression struct {
	Condition []struct {
		DBName string    `json:"DBName"`
		Value  []*string `json:"Value"`
	} `json:"Condition"`
	Operation string `json:"Operation"`
	SortOrder []struct {
		Field     string `json:"Field"`
		Direction string `json:"Direction"`
	} `json:"SortOrder"`
}

func (s *Server) dialogExpression(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.lastExpression = body
	s.lastQuery = r.URL.Query()
	s.mu.Unlock()
	if r.URL.Query().Get("dialogId") != "dlg-search" {
		writeJSON(w, 400, map[string]any{"Message": "dialogId missing"})
		return
	}
	var expr expression
	if err := json.Unmarshal(body, &expr); err != nil {
		writeJSON(w, 400, map[string]any{"Message": "bad expression"})
		return
	}
	var hits []doc
	for _, d := range docs {
		match := expr.Operation != "Or"
		for _, c := range expr.Condition {
			ok := matchCondition(d, c.DBName, c.Value)
			if expr.Operation == "Or" {
				match = match || ok
			} else {
				match = match && ok
			}
		}
		if len(expr.Condition) == 0 {
			match = true
		}
		if match {
			hits = append(hits, d)
		}
	}
	for i := len(expr.SortOrder) - 1; i >= 0; i-- {
		so := expr.SortOrder[i]
		sort.SliceStable(hits, func(a, b int) bool {
			less := sortKey(hits[a], so.Field) < sortKey(hits[b], so.Field)
			if so.Direction == "Desc" {
				return sortKey(hits[a], so.Field) > sortKey(hits[b], so.Field)
			}
			return less
		})
	}
	s.page(w, r, hits)
}

func (s *Server) listDocuments(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.lastQuery = r.URL.Query()
	s.mu.Unlock()
	s.page(w, r, docs)
}

// page answers with at most PageSize items and a "next" link for the rest.
func (s *Server) page(w http.ResponseWriter, r *http.Request, hits []doc) {
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	start, _ := strconv.Atoi(r.URL.Query().Get("start"))
	s.mu.Lock()
	qid := strconv.Itoa(len(s.results) + 1)
	s.results[qid] = hits
	s.mu.Unlock()
	s.writePage(w, qid, hits, start, count)
}

func (s *Server) resultPage(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	hits, ok := s.results[r.PathValue("qid")]
	s.mu.Unlock()
	if !ok {
		notFound(w, "result expired")
		return
	}
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	start, _ := strconv.Atoi(r.URL.Query().Get("start"))
	s.writePage(w, r.PathValue("qid"), hits, start, count)
}

func (s *Server) writePage(w http.ResponseWriter, qid string, hits []doc, start, count int) {
	if count <= 0 {
		count = 50
	}
	end := min(start+min(count, s.PageSize), len(hits))
	start = min(start, len(hits))
	items := []map[string]any{}
	for _, d := range hits[start:end] {
		items = append(items, d.summary())
	}
	links := []link{}
	remaining := count - (end - start)
	if end < len(hits) && remaining > 0 {
		links = append(links, link{"next", fmt.Sprintf("%s/Query/Results/%s?start=%d&count=%d", fcPath(invoices), qid, end, remaining)})
	}
	writeJSON(w, 200, map[string]any{
		"Items": items,
		"Count": map[string]any{"Value": len(hits), "HasMore": end < len(hits)},
		"Links": links,
	})
}

func sortKey(d doc, field string) string {
	switch field {
	case "INVOICE_DATE", "DWSTOREDATETIME":
		return d.date.Format("20060102")
	case "AMOUNT":
		return fmt.Sprintf("%015.2f", d.amount)
	default:
		return strings.ToLower(d.company)
	}
}

func matchCondition(d doc, field string, values []*string) bool {
	switch field {
	case "COMPANY":
		return matchText(d.company, values)
	case "STATUS":
		return matchText(d.status, values)
	case "INVOICE_DATE":
		return matchRange(values, func(v string) (float64, bool) {
			t, err := time.ParseInLocation("2006-01-02", v, time.Local)
			return float64(t.Unix()), err == nil
		}, float64(d.date.Unix()))
	case "AMOUNT":
		return matchRange(values, parseFloat, d.amount)
	case "INVOICE_NO":
		return matchRange(values, parseFloat, float64(d.number))
	}
	return false
}

func parseFloat(v string) (float64, bool) {
	f, err := strconv.ParseFloat(v, 64)
	return f, err == nil
}

func matchRange(values []*string, parse func(string) (float64, bool), actual float64) bool {
	switch len(values) {
	case 1:
		v, ok := parse(*values[0])
		return ok && v == actual
	case 2:
		if values[0] != nil {
			if lo, ok := parse(*values[0]); !ok || actual < lo {
				return false
			}
		}
		if values[1] != nil {
			if hi, ok := parse(*values[1]); !ok || actual > hi {
				return false
			}
		}
		return true
	}
	return false
}

func matchText(actual string, values []*string) bool {
	for _, v := range values {
		if v == nil {
			continue
		}
		switch *v {
		case "EMPTY()":
			if actual == "" {
				return true
			}
			continue
		case "NOTEMPTY()":
			if actual != "" {
				return true
			}
			continue
		}
		if wildcard(*v).MatchString(actual) {
			return true
		}
	}
	return false
}

// wildcard converts a DocuWare pattern (* ?, backslash escapes) to a regexp.
func wildcard(p string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("(?i)^")
	for i := 0; i < len(p); i++ {
		switch c := p[i]; {
		case c == '\\' && i+1 < len(p):
			i++
			b.WriteString(regexp.QuoteMeta(string(p[i])))
		case c == '*':
			b.WriteString(".*")
		case c == '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func (s *Server) sectionData(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.lastQuery = r.URL.Query()
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/pdf")
	fmt.Fprintf(w, "%%PDF-1.4 fake section %s", r.PathValue("sid"))
}
