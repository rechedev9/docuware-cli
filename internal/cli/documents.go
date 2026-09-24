package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rechedev9/docuware-cli/internal/docuware"
)

const defaultColumns = 8

func (a *app) searchCmd() *cobra.Command {
	var q docuware.Query
	var dialog, columns string
	var allFields bool
	cmd := &cobra.Command{
		Use:   "search <cabinet> [FIELD=VALUE ...]",
		Short: "Search documents in a file cabinet",
		Long: `Search documents with the cabinet's search dialog.

Conditions:
  FIELD=VALUE      exact match; * and ? are wildcards (COMPANY=Peters*)
  FIELD=FROM..TO   range on number and date fields (INVOICE_DATE=2024-01-01..2024-03-31)
  FIELD>=VALUE     lower bound; FIELD<=VALUE upper bound (number and date fields)
  FIELD=EMPTY()    the field is empty; FIELD=NOTEMPTY() has a value

FIELD is the database name or the label shown by "dw fields"; dates use
YYYY-MM-DD. Conditions are combined with AND unless --or is given; a field
may repeat only with --or (STATUS=Open STATUS=Overdue --or). DocuWare has no
mixed AND/OR, so run one search per value when you need both. Without
conditions the documents are listed as stored.`,
		Example: `  dw search Invoices COMPANY=Peters* STATUS=Open
  dw search Invoices "AMOUNT>=1000" INVOICE_DATE=2024-01-01..2024-12-31 --sort INVOICE_DATE:desc
  dw search Invoices STATUS=Open STATUS=Overdue --or --limit 50 --json`,
		Args: minArgs(1, "a file cabinet name or id, then conditions"),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := a.connect(ctx)
			if err != nil {
				return err
			}
			fc, err := c.FileCabinet(ctx, args[0])
			if err != nil {
				return err
			}
			q.Conditions = args[1:]
			var dlg *docuware.Dialog
			if len(q.Conditions) > 0 || len(q.Sort) > 0 {
				d, err := c.SearchDialog(ctx, fc, dialog)
				if err != nil {
					return err
				}
				dlg = &d
			}
			res, err := c.Search(ctx, fc, dlg, q)
			if err != nil {
				return err
			}
			if a.jsonOut {
				type item struct {
					ID     int64          `json:"id"`
					Title  string         `json:"title,omitempty"`
					Fields map[string]any `json:"fields"`
				}
				items := make([]item, 0, len(res.Items))
				for _, d := range res.Items {
					items = append(items, item{ID: d.DocumentID(), Title: d.Title, Fields: fieldMap(d.Fields, allFields)})
				}
				out := map[string]any{
					"cabinet":  map[string]string{"id": fc.ID, "name": fc.Name},
					"total":    res.Total,
					"offset":   q.Offset,
					"has_more": res.HasMore,
					"items":    items,
				}
				if res.HasMore {
					out["next_offset"] = q.Offset + len(res.Items)
				}
				return a.printJSON(out)
			}
			a.printSearch(fc, q, res, columns, allFields)
			return nil
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&dialog, "dialog", "d", "", "search dialog name or id (default: the cabinet's default)")
	fl.BoolVar(&q.Or, "or", false, "match any condition instead of all")
	fl.StringArrayVarP(&q.Sort, "sort", "s", nil, "sort by FIELD, FIELD:asc or FIELD:desc (repeatable)")
	fl.IntVarP(&q.Limit, "limit", "n", 20, "maximum number of documents")
	fl.IntVar(&q.Offset, "offset", 0, "skip this many documents (for paging)")
	fl.StringVarP(&columns, "columns", "c", "", "comma-separated fields to show (text output)")
	fl.BoolVar(&allFields, "all-fields", false, "include DocuWare system fields")
	return cmd
}

func (a *app) printSearch(fc docuware.FileCabinet, q docuware.Query, res docuware.SearchResult, columns string, allFields bool) {
	if len(res.Items) == 0 {
		fprintf(a.stdout, "%s: no documents found\n", fc.Name)
		return
	}
	cols := pickColumns(res.Items[0].Fields, columns, allFields)
	headers := append([]string{"ID"}, cols...)
	rows := make([][]string, 0, len(res.Items))
	for _, d := range res.Items {
		byName := map[string]docuware.FieldValue{}
		for _, f := range d.Fields {
			byName[strings.ToUpper(f.FieldName)] = f
		}
		row := []string{strconv.FormatInt(d.DocumentID(), 10)}
		for _, col := range cols {
			row = append(row, cell(docuware.FormatValue(byName[strings.ToUpper(col)].Value()), 40))
		}
		rows = append(rows, row)
	}
	from, to := q.Offset+1, q.Offset+len(res.Items)
	summary := fmt.Sprintf("%s: documents %d-%d", fc.Name, from, to)
	if res.Total > 0 {
		summary += fmt.Sprintf(" of %d", res.Total)
	}
	fprintf(a.stdout, "%s\n\n", summary)
	a.table(headers, rows)
	if res.HasMore {
		fprintf(a.stdout, "\nMore results: add --offset %d\n", to)
	}
}

// pickColumns returns the requested columns, or the first non-system fields.
func pickColumns(fields []docuware.FieldValue, columns string, allFields bool) []string {
	if columns != "" {
		var cols []string
		for _, c := range strings.Split(columns, ",") {
			c = strings.TrimSpace(c)
			for _, f := range fields {
				if strings.EqualFold(f.FieldName, c) || strings.EqualFold(f.FieldLabel, c) {
					c = f.FieldName
					break
				}
			}
			if c != "" {
				cols = append(cols, c)
			}
		}
		return cols
	}
	var cols []string
	for _, f := range fields {
		if f.SystemField && !allFields {
			continue
		}
		if len(cols) == defaultColumns {
			break
		}
		cols = append(cols, f.FieldName)
	}
	return cols
}

func fieldMap(fields []docuware.FieldValue, allFields bool) map[string]any {
	m := make(map[string]any, len(fields))
	for _, f := range fields {
		if f.SystemField && !allFields {
			continue
		}
		m[f.FieldName] = f.Value()
	}
	return m
}

// loadDocument resolves "<cabinet> <id>" arguments.
func (a *app) loadDocument(cmd *cobra.Command, cabinet, id string) (*docuware.Client, docuware.FileCabinet, docuware.Document, error) {
	docID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || docID <= 0 {
		return nil, docuware.FileCabinet{}, docuware.Document{}, usageErr("document id must be a positive number, got %q", id)
	}
	ctx := cmd.Context()
	c, err := a.connect(ctx)
	if err != nil {
		return nil, docuware.FileCabinet{}, docuware.Document{}, err
	}
	fc, err := c.FileCabinet(ctx, cabinet)
	if err != nil {
		return nil, docuware.FileCabinet{}, docuware.Document{}, err
	}
	doc, err := c.Document(ctx, fc.ID, docID)
	return c, fc, doc, err
}

func formatTime(s string) string {
	if t, ok := docuware.ParseTime(s); ok {
		return t.Format(time.RFC3339)
	}
	return ""
}

func (a *app) getCmd() *cobra.Command {
	var allFields bool
	cmd := &cobra.Command{
		Use:   "get <cabinet> <id>",
		Short: "Show a document's index fields and files",
		Args:  exactArgs(2, "a file cabinet and a document id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fc, doc, err := a.loadDocument(cmd, args[0], args[1])
			if err != nil {
				return err
			}
			type section struct {
				ID          string `json:"id"`
				FileName    string `json:"file_name"`
				ContentType string `json:"content_type"`
				Size        int64  `json:"size"`
				Pages       int    `json:"pages"`
			}
			sections := make([]section, 0, len(doc.Sections))
			for _, s := range doc.Sections {
				sections = append(sections, section{ID: s.ID, FileName: s.OriginalFileName, ContentType: s.ContentType, Size: s.FileSize, Pages: s.PageCount})
			}
			if a.jsonOut {
				return a.printJSON(map[string]any{
					"id":           doc.DocumentID(),
					"cabinet":      map[string]string{"id": fc.ID, "name": fc.Name},
					"title":        doc.Title,
					"content_type": doc.ContentType,
					"size":         doc.FileSize,
					"created":      formatTime(doc.CreatedAt),
					"modified":     formatTime(doc.LastModified),
					"fields":       fieldMap(doc.Fields, allFields),
					"sections":     sections,
				})
			}
			fprintf(a.stdout, "Document %d in %s\n", doc.DocumentID(), fc.Name)
			if doc.Title != "" {
				fprintf(a.stdout, "Title:     %s\n", doc.Title)
			}
			if t := formatTime(doc.CreatedAt); t != "" {
				fprintf(a.stdout, "Created:   %s\n", t)
			}
			if t := formatTime(doc.LastModified); t != "" {
				fprintf(a.stdout, "Modified:  %s\n", t)
			}
			fprintf(a.stdout, "\n")
			var rows [][]string
			for _, f := range doc.Fields {
				if f.SystemField && !allFields {
					continue
				}
				rows = append(rows, []string{f.FieldName, cell(docuware.FormatValue(f.Value()), 200)})
			}
			a.table([]string{"FIELD", "VALUE"}, rows)
			if len(sections) > 0 {
				fprintf(a.stdout, "\n")
				rows = rows[:0]
				for _, s := range sections {
					rows = append(rows, []string{s.ID, s.FileName, s.ContentType, strconv.Itoa(s.Pages), humanSize(s.Size)})
				}
				a.table([]string{"SECTION", "FILE", "TYPE", "PAGES", "SIZE"}, rows)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&allFields, "all-fields", false, "include DocuWare system fields")
	return cmd
}

func (a *app) textCmd() *cobra.Command {
	var sectionID string
	var maxChars int
	cmd := &cobra.Command{
		Use:   "text <cabinet> <id>",
		Short: "Print the OCR fulltext of a document",
		Long: `Print the text DocuWare extracted from the document (its "textshot").
The cabinet must be fulltext-indexed. Output is capped by --max-chars to keep
agent context small; use --max-chars 0 for everything.`,
		Args: exactArgs(2, "a file cabinet and a document id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, fc, doc, err := a.loadDocument(cmd, args[0], args[1])
			if err != nil {
				return err
			}
			sections := doc.Sections
			if sectionID != "" {
				sections = nil
				for _, s := range doc.Sections {
					if s.ID == sectionID {
						sections = append(sections, s)
					}
				}
				if len(sections) == 0 {
					return &docuware.NotFoundError{Kind: "section", Key: sectionID}
				}
			}
			type result struct {
				ID        string `json:"id"`
				FileName  string `json:"file_name"`
				Text      string `json:"text,omitempty"`
				Truncated bool   `json:"truncated,omitempty"`
				Error     string `json:"error,omitempty"`
			}
			budget := maxChars
			results := make([]result, 0, len(sections))
			found := false
			for _, s := range sections {
				r := result{ID: s.ID, FileName: s.OriginalFileName}
				text, err := c.SectionText(cmd.Context(), fc.ID, s)
				switch {
				case errors.Is(err, docuware.ErrNoText):
					r.Error = err.Error()
				case err != nil:
					return err
				default:
					found = true
					if maxChars > 0 {
						text, r.Truncated = truncateRunes(text, budget)
						budget = max(budget-len([]rune(text)), 0)
					}
					r.Text = text
				}
				results = append(results, r)
			}
			if !found {
				return docuware.ErrNoText
			}
			if a.jsonOut {
				return a.printJSON(map[string]any{"id": doc.DocumentID(), "cabinet": fc.Name, "sections": results})
			}
			truncated := false
			for i, r := range results {
				if len(results) > 1 {
					if i > 0 {
						fprintf(a.stdout, "\n")
					}
					fprintf(a.stdout, "=== %s (section %s) ===\n", r.FileName, r.ID)
				}
				if r.Error != "" {
					fprintf(a.stdout, "[%s]\n", r.Error)
					continue
				}
				fprintf(a.stdout, "%s\n", r.Text)
				truncated = truncated || r.Truncated
			}
			if truncated {
				fprintf(a.stderr, "note: text cut at %d characters; use --max-chars 0 for everything\n", maxChars)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&sectionID, "section", "", "only this section id (see dw get)")
	cmd.Flags().IntVar(&maxChars, "max-chars", 20000, "maximum characters to print; 0 = no limit")
	return cmd
}

func truncateRunes(s string, n int) (string, bool) {
	r := []rune(s)
	if len(r) <= n {
		return s, false
	}
	return string(r[:n]), true
}

func (a *app) downloadCmd() *cobra.Command {
	var output, sectionID string
	var pdf, annotations, force bool
	cmd := &cobra.Command{
		Use:   "download <cabinet> <id>",
		Short: "Download a document's file",
		Long: `Download the document in its stored format, or as PDF with --pdf.
Documents with several files arrive as a single archive unless --section picks one.
--output may be a file, a directory, or "-" for stdout. Existing files are
never overwritten unless --force is given; a numbered name is used instead.`,
		Example: `  dw download Invoices 42
  dw download Invoices 42 --pdf --annotations -o ./out/
  dw download Invoices 42 -o - | pdftotext - -`,
		Args: exactArgs(2, "a file cabinet and a document id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, doc, err := a.loadDocument(cmd, args[0], args[1])
			if err != nil {
				return err
			}
			d, err := c.Download(cmd.Context(), doc, docuware.DownloadOptions{PDF: pdf, Annotations: annotations, SectionID: sectionID})
			if err != nil {
				return err
			}
			defer d.Body.Close()
			if output == "-" {
				_, err := io.Copy(a.stdout, d.Body)
				return err
			}
			target, err := downloadTarget(output, d.FileName, force)
			if err != nil {
				return err
			}
			n, err := writeNewFile(target, d.Body, force)
			if err != nil {
				return err
			}
			if abs, err := filepath.Abs(target); err == nil {
				target = abs
			}
			if a.jsonOut {
				return a.printJSON(map[string]any{"path": target, "bytes": n, "content_type": d.ContentType})
			}
			fprintf(a.stdout, "Saved %s (%s)\n", target, humanSize(n))
			return nil
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&output, "output", "o", "", "file, directory, or - for stdout (default: current directory)")
	fl.BoolVar(&pdf, "pdf", false, "convert to PDF")
	fl.BoolVar(&annotations, "annotations", false, "burn annotations and stamps into a PDF")
	fl.StringVar(&sectionID, "section", "", "download only this section id (see dw get)")
	fl.BoolVar(&force, "force", false, "overwrite an existing file")
	return cmd
}

func downloadTarget(output, fileName string, force bool) (string, error) {
	target := fileName
	if output != "" {
		if fi, err := os.Stat(output); (err == nil && fi.IsDir()) || strings.HasSuffix(output, "/") || strings.HasSuffix(output, `\`) {
			if err := os.MkdirAll(output, 0o755); err != nil {
				return "", err
			}
			target = filepath.Join(output, fileName)
		} else {
			target = output
		}
	}
	if force {
		return target, nil
	}
	return uniquePath(target)
}

func uniquePath(p string) (string, error) {
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	ext := filepath.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", stem, i, ext)
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("too many files named like %s", p)
}

func writeNewFile(path string, r io.Reader, force bool) (int64, error) {
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return n, err
	}
	return n, nil
}
