package docuware

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

func cabinetPath(id string) string {
	return PlatformPath + "/FileCabinets/" + url.PathEscape(id)
}

// Version returns the DocuWare Platform version.
func (c *Client) Version(ctx context.Context) (string, error) {
	var root struct {
		Version string `json:"Version"`
	}
	if err := c.getJSON(ctx, PlatformPath, nil, &root); err != nil {
		return "", err
	}
	return root.Version, nil
}

// Organizations lists the organizations the user belongs to.
func (c *Client) Organizations(ctx context.Context) ([]Organization, error) {
	var list struct {
		Organization []Organization `json:"Organization"`
	}
	err := c.getCached(ctx, PlatformPath+"/Organizations", &list)
	return list.Organization, err
}

// FileCabinets lists file cabinets and document trays (baskets).
func (c *Client) FileCabinets(ctx context.Context) ([]FileCabinet, error) {
	var list struct {
		FileCabinet []FileCabinet `json:"FileCabinet"`
	}
	err := c.getCached(ctx, PlatformPath+"/FileCabinets", &list)
	return list.FileCabinet, err
}

// FileCabinet finds a file cabinet or basket by id or name (case-insensitive).
func (c *Client) FileCabinet(ctx context.Context, key string) (FileCabinet, error) {
	all, err := c.FileCabinets(ctx)
	if err != nil {
		return FileCabinet{}, err
	}
	for _, fc := range all {
		if strings.EqualFold(fc.ID, key) || strings.EqualFold(fc.Name, key) {
			return fc, nil
		}
	}
	names := make([]string, 0, len(all))
	for _, fc := range all {
		names = append(names, fc.Name)
	}
	return FileCabinet{}, &NotFoundError{Kind: "file cabinet", Key: key, Available: names}
}

// Dialogs lists the dialogs of a file cabinet that the user can use.
func (c *Client) Dialogs(ctx context.Context, fc FileCabinet) ([]DialogInfo, error) {
	var list struct {
		Dialog []DialogInfo `json:"Dialog"`
	}
	p := fc.Links.Href("dialogs")
	if p == "" {
		p = cabinetPath(fc.ID) + "/Dialogs"
	}
	if err := c.getCached(ctx, p, &list); err != nil {
		return nil, err
	}
	out := list.Dialog[:0]
	for _, d := range list.Dialog {
		// Same filter as the reference clients: skip non-dialog entries and
		// internal dialogs, whose ids contain an underscore.
		if (d.TypeTag == "" || d.TypeTag == "DialogInfo") && !strings.Contains(d.ID, "_") {
			out = append(out, d)
		}
	}
	return out, nil
}

// SearchDialog loads a search dialog by id or name, or the default one when key is empty.
func (c *Client) SearchDialog(ctx context.Context, fc FileCabinet, key string) (Dialog, error) {
	infos, err := c.Dialogs(ctx, fc)
	if err != nil {
		return Dialog{}, err
	}
	var search []DialogInfo
	for _, d := range infos {
		if strings.EqualFold(d.Type, "Search") {
			search = append(search, d)
		}
	}
	if len(search) == 0 {
		return Dialog{}, &NotFoundError{Kind: "search dialog", Key: fc.Name}
	}
	chosen := -1
	for i, d := range search {
		if key != "" && (strings.EqualFold(d.ID, key) || strings.EqualFold(d.DisplayName, key)) {
			chosen = i
			break
		}
		if key == "" && d.IsDefault && chosen < 0 {
			chosen = i
		}
	}
	if chosen < 0 {
		if key != "" {
			names := make([]string, 0, len(search))
			for _, d := range search {
				names = append(names, d.DisplayName)
			}
			return Dialog{}, &NotFoundError{Kind: "search dialog", Key: key, Available: names}
		}
		chosen = 0
	}
	info := search[chosen]
	p := info.Links.Href("self")
	if p == "" {
		p = cabinetPath(fc.ID) + "/Dialogs/" + url.PathEscape(info.ID)
	}
	var dlg Dialog
	if err := c.getCached(ctx, p, &dlg); err != nil {
		return Dialog{}, err
	}
	if dlg.ID == "" {
		dlg.ID = info.ID
	}
	if dlg.DisplayName == "" {
		dlg.DisplayName = info.DisplayName
	}
	return dlg, nil
}

// SelectList returns the values DocuWare suggests for a dialog field.
func (c *Client) SelectList(ctx context.Context, f DialogField) ([]any, error) {
	href := f.Links.Href("simpleSelectList")
	if href == "" {
		return nil, fmt.Errorf("field %s has no value list", f.DBFieldName)
	}
	var res struct {
		Value []any `json:"Value"`
	}
	err := c.getJSON(ctx, href, nil, &res)
	return res.Value, err
}

// maxPageSize is the largest count DocuWare accepts for one result page.
const maxPageSize = 10000

// SearchResult is the outcome of Search.
type SearchResult struct {
	Total   int
	HasMore bool
	Items   []Document
}

// Search runs q against the file cabinet. dlg may be nil when q has no
// conditions and no sort, in which case documents are listed as stored.
func (c *Client) Search(ctx context.Context, fc FileCabinet, dlg *Dialog, q Query) (SearchResult, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	params := url.Values{
		// DocuWare Cloud refuses larger pages (KBA-36909); next links cover the rest.
		"count": {strconv.Itoa(min(limit, maxPageSize))},
		"start": {strconv.Itoa(max(q.Offset, 0))},
	}
	var page QueryResult
	if len(q.Conditions) == 0 && len(q.Sort) == 0 {
		if err := c.getJSON(ctx, cabinetPath(fc.ID)+"/Query/Documents", params, &page); err != nil {
			return SearchResult{}, err
		}
	} else {
		if dlg == nil {
			return SearchResult{}, queryErr("a search dialog is required for conditions or sorting")
		}
		expr, err := BuildExpression(dlg.Fields, q)
		if err != nil {
			return SearchResult{}, err
		}
		target := dlg.Query.Links.Href("dialogExpression")
		if target == "" {
			target = cabinetPath(fc.ID) + "/Query/DialogExpression?dialogId=" + url.QueryEscape(dlg.ID)
		}
		if err := c.postJSON(ctx, target, params, expr, &page); err != nil {
			return SearchResult{}, err
		}
	}

	res := SearchResult{Total: page.Count.Value, Items: page.Items}
	for len(res.Items) < limit {
		next := page.next()
		if next == "" {
			break
		}
		page = QueryResult{}
		if err := c.getJSON(ctx, next, nil, &page); err != nil {
			return res, err
		}
		if len(page.Items) == 0 {
			break
		}
		res.Items = append(res.Items, page.Items...)
	}
	truncated := len(res.Items) > limit
	if truncated {
		res.Items = res.Items[:limit]
	}
	switch {
	case res.Total > 0:
		res.HasMore = truncated || res.Total > q.Offset+len(res.Items)
	default: // total unknown: trust the last page
		res.HasMore = truncated || page.Count.HasMore || page.next() != ""
	}
	return res, nil
}

// Document loads a document with its fields and sections.
func (c *Client) Document(ctx context.Context, cabinetID string, id int64) (Document, error) {
	var doc Document
	err := c.getJSON(ctx, cabinetPath(cabinetID)+"/Documents/"+strconv.FormatInt(id, 10), nil, &doc)
	if doc.FileCabinetID == "" {
		doc.FileCabinetID = cabinetID
	}
	return doc, err
}

// DownloadOptions select the file format of a download.
type DownloadOptions struct {
	// PDF converts the document to PDF instead of the stored format.
	PDF bool
	// Annotations burns annotations and stamps into the PDF.
	Annotations bool
	// SectionID downloads a single section instead of the whole document.
	SectionID string
}

// Download is an open file download; the caller must close Body.
type Download struct {
	Body        io.ReadCloser
	FileName    string
	ContentType string
	Size        int64
}

// Download opens the document (or one of its sections) for reading.
func (c *Client) Download(ctx context.Context, doc Document, o DownloadOptions) (*Download, error) {
	href := doc.Links.Href("fileDownload")
	if href == "" {
		href = cabinetPath(doc.FileCabinetID) + "/Documents/" + strconv.FormatInt(doc.DocumentID(), 10) + "/FileDownload"
	}
	fallbackName := fmt.Sprintf("document-%d", doc.DocumentID())
	if o.SectionID != "" {
		sec, err := c.section(ctx, doc, o.SectionID)
		if err != nil {
			return nil, err
		}
		if href = sec.Links.Href("fileDownload"); href == "" {
			return nil, fmt.Errorf("section %s offers no download link", o.SectionID)
		}
		if sec.OriginalFileName != "" {
			fallbackName = sec.OriginalFileName
		}
	}
	target := "Auto"
	if o.PDF || o.Annotations {
		target = "PDF"
	}
	q := url.Values{
		"targetFileType":  {target},
		"keepAnnotations": {strconv.FormatBool(o.Annotations)},
	}
	resp, err := c.do(ctx, request{method: http.MethodGet, path: href, query: q, accept: "*/*"})
	if err != nil {
		return nil, err
	}
	name := fileNameFromDisposition(resp.Header.Get("Content-Disposition"))
	if name == "" {
		name = fallbackName
		if path.Ext(name) == "" {
			name += extensionFor(resp.Header.Get("Content-Type"))
		}
	}
	return &Download{
		Body:        resp.Body,
		FileName:    SanitizeFileName(name, fallbackName),
		ContentType: resp.Header.Get("Content-Type"),
		Size:        resp.ContentLength,
	}, nil
}

// section returns a section with its full set of links.
func (c *Client) section(ctx context.Context, doc Document, id string) (Section, error) {
	for _, s := range doc.Sections {
		if s.ID == id {
			return c.loadSection(ctx, doc.FileCabinetID, s, "fileDownload")
		}
	}
	ids := make([]string, 0, len(doc.Sections))
	for _, s := range doc.Sections {
		ids = append(ids, s.ID)
	}
	return Section{}, &NotFoundError{Kind: "section", Key: id, Available: ids}
}

// loadSection fetches the section itself when the embedded copy lacks rel.
func (c *Client) loadSection(ctx context.Context, cabinetID string, s Section, rel string) (Section, error) {
	if s.Links.Href(rel) != "" {
		return s, nil
	}
	self := s.Links.Href("self")
	if self == "" {
		self = cabinetPath(cabinetID) + "/Sections/" + url.PathEscape(s.ID)
	}
	var full Section
	if err := c.getJSON(ctx, self, nil, &full); err != nil {
		return Section{}, err
	}
	return full, nil
}

// SectionText returns the OCR fulltext of a section as plain text.
func (c *Client) SectionText(ctx context.Context, cabinetID string, s Section) (string, error) {
	full, err := c.loadSection(ctx, cabinetID, s, "textshot")
	if err != nil {
		return "", err
	}
	href := full.Links.Href("textshot")
	if href == "" {
		return "", ErrNoText
	}
	raw, err := c.Get(ctx, href, nil)
	if err != nil {
		return "", err
	}
	return TextFromTextshot(raw)
}

func fileNameFromDisposition(h string) string {
	if h == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(h)
	if err != nil {
		return ""
	}
	return params["filename"]
}

func extensionFor(contentType string) string {
	mt, _, _ := mime.ParseMediaType(contentType)
	switch mt {
	case "application/pdf":
		return ".pdf"
	case "application/zip", "application/x-zip-compressed":
		return ".zip"
	}
	if exts, _ := mime.ExtensionsByType(mt); len(exts) > 0 {
		return exts[0]
	}
	return ""
}

// SanitizeFileName reduces a server-supplied name to a safe base name.
func SanitizeFileName(name, fallback string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	name = path.Base(name)
	if i := strings.LastIndexByte(name, ':'); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(strings.TrimSpace(name), ". ")
	if name == "" || name == "." || name == ".." {
		return fallback
	}
	return name
}
