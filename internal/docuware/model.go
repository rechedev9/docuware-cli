package docuware

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// Link is a hypermedia link in a DocuWare response.
type Link struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}

// Links is the "Links" array most DocuWare resources carry.
type Links []Link

// Href returns the href for rel (case-insensitive), or "".
func (ls Links) Href(rel string) string {
	for _, l := range ls {
		if strings.EqualFold(l.Rel, rel) {
			return l.Href
		}
	}
	return ""
}

// Organization is a DocuWare organization.
type Organization struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	Links Links  `json:"Links"`
}

// FileCabinet is a file cabinet or, when IsBasket is set, a document tray.
type FileCabinet struct {
	ID       string `json:"Id"`
	Name     string `json:"Name"`
	IsBasket bool   `json:"IsBasket"`
	Default  bool   `json:"Default"`
	Links    Links  `json:"Links"`
}

// DialogInfo is an entry of a file cabinet's dialog list.
type DialogInfo struct {
	TypeTag       string `json:"$type"`
	ID            string `json:"Id"`
	DisplayName   string `json:"DisplayName"`
	Type          string `json:"Type"`
	IsDefault     bool   `json:"IsDefault"`
	FileCabinetID string `json:"FileCabinetId"`
	Links         Links  `json:"Links"`
}

// Dialog is a fully loaded dialog, including its fields.
type Dialog struct {
	ID            string        `json:"Id"`
	DisplayName   string        `json:"DisplayName"`
	Type          string        `json:"Type"`
	FileCabinetID string        `json:"FileCabinetId"`
	IsDefault     bool          `json:"IsDefault"`
	Fields        []DialogField `json:"Fields"`
	Query         struct {
		Links Links `json:"Links"`
	} `json:"Query"`
	Links Links `json:"Links"`
}

// DialogField is a field of a search or store dialog.
type DialogField struct {
	DBFieldName string `json:"DBFieldName"`
	DlgLabel    string `json:"DlgLabel"`
	DWFieldType string `json:"DWFieldType"`
	Length      int    `json:"Length"`
	Links       Links  `json:"Links"`
}

// Label returns the display label, falling back to the database name.
func (f DialogField) Label() string {
	if f.DlgLabel != "" {
		return f.DlgLabel
	}
	return f.DBFieldName
}

// HasSelectList reports whether DocuWare offers a value list for the field.
func (f DialogField) HasSelectList() bool { return f.Links.Href("simpleSelectList") != "" }

// FieldValue is an index field value of a document.
type FieldValue struct {
	FieldName       string          `json:"FieldName"`
	FieldLabel      string          `json:"FieldLabel"`
	ItemElementName string          `json:"ItemElementName"`
	Item            json.RawMessage `json:"Item"`
	IsNull          bool            `json:"IsNull"`
	ReadOnly        bool            `json:"ReadOnly"`
	SystemField     bool            `json:"SystemField"`
}

// Section is one file ("attachment") of a document.
type Section struct {
	ID               string `json:"Id"`
	ContentType      string `json:"ContentType"`
	OriginalFileName string `json:"OriginalFileName"`
	FileSize         int64  `json:"FileSize"`
	PageCount        int    `json:"PageCount"`
	Links            Links  `json:"Links"`
}

// DocID is a document id. DocuWare sends it as a JSON number.
type DocID int64

// UnmarshalJSON accepts both numbers and numeric strings.
func (d *DocID) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*d = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*d = DocID(n)
	return nil
}

// Document is a document or a search result item.
type Document struct {
	ID            DocID        `json:"Id"`
	Title         string       `json:"Title"`
	ContentType   string       `json:"ContentType"`
	FileSize      int64        `json:"FileSize"`
	FileCabinetID string       `json:"FileCabinetId"`
	LastModified  string       `json:"LastModified"`
	CreatedAt     string       `json:"CreatedAt"`
	Fields        []FieldValue `json:"Fields"`
	Sections      []Section    `json:"Sections"`
	Links         Links        `json:"Links"`
}

// DocumentID returns the id, falling back to the DWDOCID system field.
func (d Document) DocumentID() int64 {
	if d.ID != 0 {
		return int64(d.ID)
	}
	for _, f := range d.Fields {
		if strings.EqualFold(f.FieldName, "DWDOCID") {
			if n, ok := f.Value().(int64); ok {
				return n
			}
		}
	}
	return 0
}

// QueryResult is one page of search results.
type QueryResult struct {
	Items []Document `json:"Items"`
	Count Count      `json:"Count"`
	Next  string     `json:"Next"`
	Links Links      `json:"Links"`
}

func (q QueryResult) next() string {
	if href := q.Links.Href("next"); href != "" {
		return href
	}
	return q.Next
}

// Count is the total hit count. DocuWare sends {"Value": n, "HasMore": b};
// a bare number is accepted too.
type Count struct {
	Value   int
	HasMore bool
}

// UnmarshalJSON accepts both the object and the bare-number form.
func (c *Count) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '{' {
		var v struct {
			Value   int  `json:"Value"`
			HasMore bool `json:"HasMore"`
		}
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		c.Value, c.HasMore = v.Value, v.HasMore
		return nil
	}
	return json.Unmarshal(b, &c.Value)
}
