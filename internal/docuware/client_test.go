package docuware_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/rechedev9/docuware-cli/internal/docuware"
	"github.com/rechedev9/docuware-cli/internal/dwfake"
)

func newClient(t *testing.T, fake *dwfake.Server, creds docuware.Credentials) *docuware.Client {
	t.Helper()
	c, err := docuware.New(docuware.Options{URL: fake.URL, Credentials: creds})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func passwordCreds(fake *dwfake.Server) docuware.Credentials {
	return docuware.Credentials{Method: docuware.MethodPassword, Username: fake.Username, Password: fake.Password}
}

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"acme":                "https://acme.docuware.cloud",
		"acme.docuware.cloud": "https://acme.docuware.cloud",
		"https://acme.docuware.cloud/DocuWare/Platform/": "https://acme.docuware.cloud",
		"dms.example.com/DocuWare/Platform":              "https://dms.example.com",
		"http://dwserver:8080":                           "http://dwserver:8080",
		"localhost:9000":                                 "https://localhost:9000",
	}
	for in, want := range cases {
		got, err := docuware.NormalizeURL(in)
		if err != nil || got != want {
			t.Errorf("NormalizeURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://x.example.com"} {
		if _, err := docuware.NormalizeURL(bad); err == nil {
			t.Errorf("NormalizeURL(%q) should fail", bad)
		}
	}
}

func TestLoginDiscoversTokenEndpointAndReusesToken(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	var saved []docuware.Token
	c, err := docuware.New(docuware.Options{URL: fake.URL, Credentials: passwordCreds(fake), OnToken: func(tok docuware.Token) { saved = append(saved, tok) }})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tok, err := c.Login(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken == "" || !strings.HasSuffix(tok.TokenEndpoint, "/identity/connect/token") {
		t.Fatalf("unexpected token %+v", tok)
	}
	if _, err := c.FileCabinets(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Version(ctx); err != nil {
		t.Fatal(err)
	}
	if n := fake.TokenRequests(); n != 1 {
		t.Errorf("token requests = %d, want 1", n)
	}
	if len(saved) != 1 {
		t.Errorf("OnToken called %d times, want 1", len(saved))
	}
}

func TestCachedTokenSkipsLogin(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	ctx := context.Background()
	first := newClient(t, fake, passwordCreds(fake))
	tok, err := first.Login(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// A new process with the cached token and no credentials still works.
	c, _ := docuware.New(docuware.Options{URL: fake.URL, Token: tok})
	if _, err := c.FileCabinets(ctx); err != nil {
		t.Fatal(err)
	}
	if n := fake.TokenRequests(); n != 1 {
		t.Errorf("token requests = %d, want 1", n)
	}
}

func TestExpiredTokenIsRenewedOnce(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	ctx := context.Background()
	c := newClient(t, fake, passwordCreds(fake))
	if _, err := c.Login(ctx); err != nil {
		t.Fatal(err)
	}
	fake.ExpireTokens()
	if _, err := c.FileCabinets(ctx); err != nil {
		t.Fatalf("request after expiry: %v", err)
	}
	if n := fake.TokenRequests(); n != 2 {
		t.Errorf("token requests = %d, want 2", n)
	}
}

func TestExpiredTokenWithoutCredentials(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	ctx := context.Background()
	tok, _ := newClient(t, fake, passwordCreds(fake)).Login(ctx)
	fake.ExpireTokens()
	c, _ := docuware.New(docuware.Options{URL: fake.URL, Token: tok})
	_, err := c.FileCabinets(ctx)
	var apiErr *docuware.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 401 {
		t.Fatalf("want 401 APIError, got %v", err)
	}
}

func TestLoginErrors(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	ctx := context.Background()

	bad := newClient(t, fake, docuware.Credentials{Method: docuware.MethodPassword, Username: "peggy", Password: "wrong"})
	var authErr *docuware.AuthError
	if _, err := bad.Login(ctx); !errors.As(err, &authErr) {
		t.Fatalf("want AuthError, got %v", err)
	}

	none := newClient(t, fake, docuware.Credentials{})
	if _, err := none.FileCabinets(ctx); !errors.Is(err, docuware.ErrNoCredentials) {
		t.Fatalf("want ErrNoCredentials, got %v", err)
	}
}

func TestClientCredentialsGrant(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	c := newClient(t, fake, docuware.Credentials{Method: docuware.MethodClientCredentials, ClientID: fake.ClientID, ClientSecret: fake.ClientSecret})
	if _, err := c.FileCabinets(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestThrottlingIsRetried(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	c := newClient(t, fake, passwordCreds(fake))
	fake.Throttle(2)
	if _, err := c.FileCabinets(context.Background()); err != nil {
		t.Fatalf("throttled request: %v", err)
	}
}

func TestRefusesToSendTokenToOtherHosts(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	c := newClient(t, fake, passwordCreds(fake))
	_, err := c.Get(context.Background(), "https://evil.example.com/steal", nil)
	if err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("want refusal, got %v", err)
	}
}

func TestFileCabinetLookup(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	c := newClient(t, fake, passwordCreds(fake))
	ctx := context.Background()
	for _, key := range []string{"invoices", "fc-inv"} {
		fc, err := c.FileCabinet(ctx, key)
		if err != nil || fc.ID != "fc-inv" {
			t.Errorf("FileCabinet(%q) = %+v, %v", key, fc, err)
		}
	}
	_, err := c.FileCabinet(ctx, "Nope")
	var nf *docuware.NotFoundError
	if !errors.As(err, &nf) || !strings.Contains(err.Error(), "Invoices") {
		t.Fatalf("want NotFoundError listing names, got %v", err)
	}
}

func TestSearchDialogSkipsInternalAndPicksDefault(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	c := newClient(t, fake, passwordCreds(fake))
	ctx := context.Background()
	fc, _ := c.FileCabinet(ctx, "Invoices")
	infos, err := c.Dialogs(ctx, fc)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range infos {
		if strings.Contains(d.ID, "_") {
			t.Errorf("internal dialog %q not filtered", d.ID)
		}
	}
	dlg, err := c.SearchDialog(ctx, fc, "")
	if err != nil || dlg.ID != "dlg-search" || len(dlg.Fields) != 5 {
		t.Fatalf("SearchDialog = %+v, %v", dlg, err)
	}
	if _, err := c.SearchDialog(ctx, fc, "Invoice search"); err != nil {
		t.Fatal(err)
	}
}

func TestSearchSendsExpressionAndDecodesFields(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	c := newClient(t, fake, passwordCreds(fake))
	ctx := context.Background()
	fc, _ := c.FileCabinet(ctx, "Invoices")
	dlg, _ := c.SearchDialog(ctx, fc, "")

	res, err := c.Search(ctx, fc, &dlg, docuware.Query{
		Conditions: []string{"company=Peters*", "Invoice date=2024-01-01..2024-12-31"},
		Sort:       []string{"AMOUNT:desc"},
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 || res.Items[0].DocumentID() != 3 || res.Items[1].DocumentID() != 1 {
		t.Fatalf("unexpected items: %+v", res.Items)
	}
	var sent map[string]any
	if err := json.Unmarshal(fake.LastExpression(), &sent); err != nil {
		t.Fatal(err)
	}
	want := `{"Condition":[{"DBName":"COMPANY","Value":["Peters*"]},{"DBName":"INVOICE_DATE","Value":["2024-01-01","2024-12-31"]}],"Operation":"And","SortOrder":[{"Direction":"Desc","Field":"AMOUNT"}]}`
	if got, _ := json.Marshal(sent); string(got) != want {
		t.Errorf("expression\n got %s\nwant %s", got, want)
	}
	if q := fake.LastQuery(); q["count"][0] != "10" || q["start"][0] != "0" || q["dialogId"][0] != "dlg-search" {
		t.Errorf("unexpected query %v", q)
	}

	values := map[string]any{}
	for _, f := range res.Items[1].Fields {
		values[f.FieldName] = f.Value()
	}
	if values["COMPANY"] != "Peters Engineering" || values["INVOICE_DATE"] != "2024-01-15" ||
		values["AMOUNT"] != 1250.5 || values["INVOICE_NO"] != int64(1001) || values["CONTACT"] != nil {
		t.Errorf("unexpected values %v", values)
	}
	if tags, _ := values["TAGS"].([]string); len(tags) != 2 || tags[0] != "urgent" {
		t.Errorf("keywords = %v", values["TAGS"])
	}
}

func TestSearchFollowsNextLinksUpToLimit(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	c := newClient(t, fake, passwordCreds(fake))
	ctx := context.Background()
	fc, _ := c.FileCabinet(ctx, "Invoices")

	// No conditions: plain listing, server pages by 2.
	res, err := c.Search(ctx, fc, nil, docuware.Query{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 3 || res.Total != 3 || res.HasMore {
		t.Fatalf("got %d items, total %d, more %v", len(res.Items), res.Total, res.HasMore)
	}

	res, err = c.Search(ctx, fc, nil, docuware.Query{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 || !res.HasMore {
		t.Fatalf("limit 2: got %d items, more %v", len(res.Items), res.HasMore)
	}
}

func TestSearchOr(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	c := newClient(t, fake, passwordCreds(fake))
	ctx := context.Background()
	fc, _ := c.FileCabinet(ctx, "Invoices")
	dlg, _ := c.SearchDialog(ctx, fc, "")
	res, err := c.Search(ctx, fc, &dlg, docuware.Query{Conditions: []string{"COMPANY=Acme Corp (EU)", "AMOUNT>=4000"}, Or: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("OR search: %d items", len(res.Items))
	}
}

func TestDocumentTextAndDownload(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	c := newClient(t, fake, passwordCreds(fake))
	ctx := context.Background()

	doc, err := c.Document(ctx, "fc-inv", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sections) != 1 {
		t.Fatalf("sections = %d", len(doc.Sections))
	}
	text, err := c.SectionText(ctx, doc.FileCabinetID, doc.Sections[0])
	if err != nil {
		t.Fatal(err)
	}
	want := "--- page 1 ---\nINVOICE 1001\nTotal\n1250.50 EUR\n\n--- page 2 ---\nPage two"
	if text != want {
		t.Errorf("text\n got %q\nwant %q", text, want)
	}

	noText, _ := c.Document(ctx, "fc-inv", 2)
	if _, err := c.SectionText(ctx, noText.FileCabinetID, noText.Sections[0]); !errors.Is(err, docuware.ErrNoText) {
		t.Errorf("want ErrNoText, got %v", err)
	}

	d, err := c.Download(ctx, doc, docuware.DownloadOptions{PDF: true})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(d.Body)
	d.Body.Close()
	if d.FileName != "invoice-1001.pdf" || !strings.HasPrefix(string(body), "%PDF") {
		t.Errorf("download = %q, %q", d.FileName, body)
	}
	if q := fake.LastQuery(); q["targetFileType"][0] != "PDF" || q["keepAnnotations"][0] != "false" {
		t.Errorf("download query %v", q)
	}

	evil, _ := c.Document(ctx, "fc-inv", 3)
	d, err = c.Download(ctx, evil, docuware.DownloadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	d.Body.Close()
	if d.FileName != "evil.pdf" {
		t.Errorf("unsafe file name not sanitized: %q", d.FileName)
	}

	d, err = c.Download(ctx, doc, docuware.DownloadOptions{SectionID: doc.Sections[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(d.Body)
	d.Body.Close()
	if !strings.Contains(string(body), "section 1-sec") || d.FileName != "invoice-1001.pdf" {
		t.Errorf("section download = %q, %q", d.FileName, body)
	}

	_, err = c.Document(ctx, "fc-inv", 99)
	var apiErr *docuware.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 404 || apiErr.Message != "Document not found" {
		t.Errorf("want 404 with message, got %v", err)
	}
}

func TestSelectList(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	c := newClient(t, fake, passwordCreds(fake))
	ctx := context.Background()
	fc, _ := c.FileCabinet(ctx, "Invoices")
	dlg, _ := c.SearchDialog(ctx, fc, "")
	f, _ := docuware.FindField(dlg.Fields, "status")
	values, err := c.SelectList(ctx, f)
	if err != nil || len(values) != 3 {
		t.Fatalf("SelectList = %v, %v", values, err)
	}
}

type memCache map[string][]byte

func (m memCache) Get(k string) ([]byte, bool) { b, ok := m[k]; return b, ok }
func (m memCache) Put(k string, b []byte)      { m[k] = b }

func TestMetadataCache(t *testing.T) {
	fake := dwfake.New()
	defer fake.Close()
	cache := memCache{}
	c, _ := docuware.New(docuware.Options{URL: fake.URL, Credentials: passwordCreds(fake), Cache: cache})
	ctx := context.Background()
	for range 3 {
		fc, err := c.FileCabinet(ctx, "Invoices")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.SearchDialog(ctx, fc, ""); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(fake.Requests()); n != 3 {
		t.Errorf("API requests = %d, want 3 (cabinets, dialogs, dialog): %v", n, fake.Requests())
	}
}
