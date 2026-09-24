package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rechedev9/docuware-cli/internal/dwfake"
)

type env map[string]string

// newEnv isolates config, cache and secrets in temp dirs.
func newEnv(t *testing.T) env {
	t.Helper()
	dir := t.TempDir()
	return env{
		"DW_CONFIG_DIR":   filepath.Join(dir, "config"),
		"DW_CACHE_DIR":    filepath.Join(dir, "cache"),
		"DW_SECRET_STORE": "file",
	}
}

func (e env) with(kv ...string) env {
	out := env{}
	for k, v := range e {
		out[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i]] = kv[i+1]
	}
	return out
}

func run(t *testing.T, e env, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(context.Background(), args, strings.NewReader(stdin), &out, &errOut, func(k string) string { return e[k] }, "test")
	return out.String(), errOut.String(), code
}

func mustRun(t *testing.T, e env, args ...string) string {
	t.Helper()
	out, errOut, code := run(t, e, "", args...)
	if code != 0 {
		t.Fatalf("dw %s: exit %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), code, out, errOut)
	}
	return out
}

func loggedIn(t *testing.T) (*dwfake.Server, env) {
	t.Helper()
	fake := dwfake.New()
	t.Cleanup(fake.Close)
	e := newEnv(t)
	out, errOut, code := run(t, e, fake.Password+"\n", "login", "--url", fake.URL, "--user", fake.Username, "--password-stdin")
	if code != 0 {
		t.Fatalf("login: exit %d: %s %s", code, out, errOut)
	}
	if !strings.Contains(out, "Logged in") || !strings.Contains(out, "7.12") {
		t.Fatalf("login output: %s", out)
	}
	return fake, e
}

func TestLoginStoresProfileAndTokenAcrossRuns(t *testing.T) {
	fake, e := loggedIn(t)
	var status struct {
		Account      string `json:"account"`
		Profile      string `json:"profile"`
		FromEnv      bool   `json:"credentials_from_env"`
		FileCabinets int    `json:"file_cabinets"`
		Baskets      int    `json:"baskets"`
	}
	if err := json.Unmarshal([]byte(mustRun(t, e, "status", "--json")), &status); err != nil {
		t.Fatal(err)
	}
	if status.Account != "peggy" || status.Profile != "default" || status.FromEnv || status.FileCabinets != 2 || status.Baskets != 1 {
		t.Errorf("status = %+v", status)
	}
	mustRun(t, e, "cabinets")
	if n := fake.TokenRequests(); n != 1 {
		t.Errorf("token requests = %d, want 1 (token reused from cache)", n)
	}

	// The stored password renews an expired token transparently.
	fake.ExpireTokens()
	mustRun(t, e, "cabinets", "--no-cache")
	if n := fake.TokenRequests(); n != 2 {
		t.Errorf("token requests after expiry = %d, want 2", n)
	}

	secrets, err := os.ReadFile(filepath.Join(e["DW_CONFIG_DIR"], "secrets.json"))
	if err != nil || !strings.Contains(string(secrets), fake.Password) {
		t.Errorf("secret not stored: %s %v", secrets, err)
	}
	cfg, _ := os.ReadFile(filepath.Join(e["DW_CONFIG_DIR"], "config.json"))
	if strings.Contains(string(cfg), fake.Password) {
		t.Error("password leaked into config.json")
	}
}

func TestEnvironmentCredentials(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	e := newEnv(t).with("DW_URL", fake.URL, "DW_USERNAME", fake.Username, "DW_PASSWORD", fake.Password)
	if out := mustRun(t, e, "status"); !strings.Contains(out, "from environment") {
		t.Errorf("status: %s", out)
	}
	e = newEnv(t).with("DW_URL", fake.URL, "DW_CLIENT_ID", fake.ClientID, "DW_CLIENT_SECRET", fake.ClientSecret)
	if out := mustRun(t, e, "status"); !strings.Contains(out, "client_credentials") {
		t.Errorf("status: %s", out)
	}
}

func TestCabinetsFieldsValues(t *testing.T) {
	_, e := loggedIn(t)
	out := mustRun(t, e, "cabinets")
	if !strings.Contains(out, "Invoices") || strings.Contains(out, "Inbox") {
		t.Errorf("cabinets: %s", out)
	}
	var all []map[string]any
	_ = json.Unmarshal([]byte(mustRun(t, e, "cabinets", "--baskets", "--json")), &all)
	if len(all) != 3 {
		t.Errorf("cabinets --baskets: %v", all)
	}
	out = mustRun(t, e, "fields", "invoices")
	for _, want := range []string{"Invoice search", "INVOICE_DATE", "Date", "STATUS"} {
		if !strings.Contains(out, want) {
			t.Errorf("fields output lacks %q:\n%s", want, out)
		}
	}
	if out := mustRun(t, e, "values", "Invoices", "Status"); out != "Open\nPaid\nOverdue\n" {
		t.Errorf("values: %q", out)
	}
	if out := mustRun(t, e, "dialogs", "Invoices"); !strings.Contains(out, "dlg-search") || strings.Contains(out, "dlg_internal") {
		t.Errorf("dialogs: %s", out)
	}
}

func TestSearch(t *testing.T) {
	_, e := loggedIn(t)
	var res struct {
		Total   int  `json:"total"`
		HasMore bool `json:"has_more"`
		Items   []struct {
			ID     int64          `json:"id"`
			Fields map[string]any `json:"fields"`
		} `json:"items"`
	}
	out := mustRun(t, e, "search", "Invoices", "COMPANY=Peters*", "--sort", "AMOUNT:desc", "--json")
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 || res.HasMore || len(res.Items) != 2 || res.Items[0].ID != 3 || res.Items[1].ID != 1 {
		t.Fatalf("search: %s", out)
	}
	if _, ok := res.Items[0].Fields["DWDOCID"]; ok {
		t.Error("system fields should be hidden without --all-fields")
	}
	if res.Items[1].Fields["INVOICE_DATE"] != "2024-01-15" {
		t.Errorf("date field: %v", res.Items[1].Fields["INVOICE_DATE"])
	}

	out = mustRun(t, e, "search", "Invoices", "--limit", "2")
	if !strings.Contains(out, "documents 1-2 of 3") || !strings.Contains(out, "--offset 2") {
		t.Errorf("paged text output:\n%s", out)
	}
	out = mustRun(t, e, "search", "Invoices", "--offset", "2", "--columns", "company,amount")
	if !strings.Contains(out, "documents 3-3 of 3") || !strings.Contains(out, "AMOUNT") || strings.Contains(out, "STATUS") {
		t.Errorf("columns output:\n%s", out)
	}
	if out := mustRun(t, e, "search", "Invoices", "STATUS=Closed"); !strings.Contains(out, "no documents found") {
		t.Errorf("empty result: %s", out)
	}
}

func TestGetTextDownload(t *testing.T) {
	_, e := loggedIn(t)
	out := mustRun(t, e, "get", "Invoices", "1")
	for _, want := range []string{"Document 1 in Invoices", "Peters Engineering", "urgent, q1", "invoice-1001.pdf"} {
		if !strings.Contains(out, want) {
			t.Errorf("get output lacks %q:\n%s", want, out)
		}
	}
	if out := mustRun(t, e, "text", "Invoices", "1"); !strings.Contains(out, "INVOICE 1001\nTotal\n1250.50 EUR") {
		t.Errorf("text: %q", out)
	}
	out, errOut, code := run(t, e, "", "text", "Invoices", "1", "--max-chars", "5")
	if code != 0 || out != "--- p\n" || !strings.Contains(errOut, "--max-chars 0") {
		t.Errorf("truncated text: %d %q %q", code, out, errOut)
	}
	if _, errOut, code := run(t, e, "", "text", "Invoices", "2"); code != exitNotFound || !strings.Contains(errOut, "dw download") {
		t.Errorf("no fulltext: exit %d, %s", code, errOut)
	}

	dir := t.TempDir() + string(os.PathSeparator)
	var dl struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal([]byte(mustRun(t, e, "download", "Invoices", "3", "-o", dir, "--json")), &dl)
	if filepath.Base(dl.Path) != "evil.pdf" || filepath.Dir(dl.Path) != filepath.Clean(dir) {
		t.Errorf("download path %q (dir %q)", dl.Path, dir)
	}
	if out := mustRun(t, e, "download", "Invoices", "3", "-o", dir); !strings.Contains(out, "evil (1).pdf") {
		t.Errorf("second download should not overwrite: %s", out)
	}
	if out := mustRun(t, e, "download", "Invoices", "1", "-o", "-"); !strings.HasPrefix(out, "%PDF") {
		t.Errorf("stdout download: %q", out)
	}
}

func TestExitCodes(t *testing.T) {
	fake, e := loggedIn(t)
	cases := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"cabinets", "extra"}, exitUsage, "expected no arguments"},
		{[]string{"nope"}, exitUsage, "unknown command"},
		{[]string{"search", "Invoices", "--bogus"}, exitUsage, "unknown flag"},
		{[]string{"search", "Nope"}, exitNotFound, "available: Invoices, Contracts, Inbox"},
		{[]string{"search", "Invoices", "NOPE=1"}, exitNotFound, "available: COMPANY"},
		{[]string{"search", "Invoices", "COMPANY>=A"}, exitUsage, "dw fields"},
		{[]string{"get", "Invoices", "abc"}, exitUsage, "positive number"},
		{[]string{"get", "Invoices", "99"}, exitNotFound, "Document not found"},
	}
	for _, tc := range cases {
		_, errOut, code := run(t, e, "", tc.args...)
		if code != tc.code || !strings.Contains(errOut, tc.want) {
			t.Errorf("dw %s: exit %d, stderr %q; want exit %d containing %q", strings.Join(tc.args, " "), code, errOut, tc.code, tc.want)
		}
	}

	if _, errOut, code := run(t, newEnv(t), "", "status"); code != exitAuth || !strings.Contains(errOut, "dw login") {
		t.Errorf("not logged in: exit %d %s", code, errOut)
	}
	if _, errOut, code := run(t, newEnv(t), "wrong\n", "login", "--url", fake.URL, "--user", "peggy", "--password-stdin"); code != exitAuth || !strings.Contains(errOut, "invalid_username_or_password") {
		t.Errorf("bad password: exit %d %s", code, errOut)
	}
	if _, _, code := run(t, newEnv(t), "", "login", "--url", fake.URL, "--user", "peggy"); code != exitUsage {
		t.Errorf("login without a terminal or stdin should be a usage error, got %d", code)
	}
}

func TestLogout(t *testing.T) {
	_, e := loggedIn(t)
	mustRun(t, e, "logout")
	if _, _, code := run(t, e, "", "status"); code != exitAuth {
		t.Errorf("status after logout: exit %d", code)
	}
	secrets, _ := os.ReadFile(filepath.Join(e["DW_CONFIG_DIR"], "secrets.json"))
	if strings.Contains(string(secrets), "s3cret") {
		t.Error("secret kept after logout")
	}
}

func TestAPIAndSkill(t *testing.T) {
	_, e := loggedIn(t)
	if out := mustRun(t, e, "api", "Organizations"); !strings.Contains(out, "Peters Engineering") {
		t.Errorf("api: %s", out)
	}
	if out := mustRun(t, e, "api", "/DocuWare/Platform/FileCabinets/fc-inv/Query/Documents", "-q", "count=1"); !strings.Contains(out, `"Items"`) {
		t.Errorf("api with query: %s", out)
	}
	dir := t.TempDir()
	mustRun(t, e, "skill", "install", "--dir", dir)
	b, err := os.ReadFile(filepath.Join(dir, "docuware", "SKILL.md"))
	if err != nil || !strings.HasPrefix(string(b), "---\nname: docuware") {
		t.Errorf("skill install: %v %q", err, b)
	}
}
