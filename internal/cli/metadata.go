package cli

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/rechedev9/docuware-cli/internal/docuware"
)

func (a *app) cabinetsCmd() *cobra.Command {
	var baskets bool
	cmd := &cobra.Command{
		Use:     "cabinets",
		Aliases: []string{"fc"},
		Short:   "List file cabinets (and document trays with --baskets)",
		Args:    exactArgs(0, "no arguments"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := a.connect(ctx)
			if err != nil {
				return err
			}
			all, err := c.FileCabinets(ctx)
			if err != nil {
				return err
			}
			type row struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Basket  bool   `json:"basket"`
				Default bool   `json:"default,omitempty"`
			}
			var rows []row
			for _, fc := range all {
				if fc.IsBasket && !baskets {
					continue
				}
				rows = append(rows, row{ID: fc.ID, Name: fc.Name, Basket: fc.IsBasket, Default: fc.Default})
			}
			if a.jsonOut {
				return a.printJSON(orEmpty(rows))
			}
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				kind := "cabinet"
				if r.Basket {
					kind = "tray"
				}
				table = append(table, []string{r.Name, kind, r.ID})
			}
			a.table([]string{"NAME", "TYPE", "ID"}, table)
			return nil
		},
	}
	cmd.Flags().BoolVar(&baskets, "baskets", false, "include document trays (baskets)")
	return cmd
}

func (a *app) dialogsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dialogs <cabinet>",
		Short: "List the dialogs of a file cabinet",
		Args:  exactArgs(1, "a file cabinet name or id"),
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
			dialogs, err := c.Dialogs(ctx, fc)
			if err != nil {
				return err
			}
			type row struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Type    string `json:"type"`
				Default bool   `json:"default,omitempty"`
			}
			rows := make([]row, 0, len(dialogs))
			for _, d := range dialogs {
				rows = append(rows, row{ID: d.ID, Name: d.DisplayName, Type: d.Type, Default: d.IsDefault})
			}
			if a.jsonOut {
				return a.printJSON(rows)
			}
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				table = append(table, []string{r.Name, r.Type, yesNo(r.Default), r.ID})
			}
			a.table([]string{"NAME", "TYPE", "DEFAULT", "ID"}, table)
			return nil
		},
	}
}

// searchDialog resolves the cabinet and its search dialog.
func (a *app) searchDialog(cmd *cobra.Command, cabinet, dialog string) (*docuware.Client, docuware.FileCabinet, docuware.Dialog, error) {
	ctx := cmd.Context()
	c, err := a.connect(ctx)
	if err != nil {
		return nil, docuware.FileCabinet{}, docuware.Dialog{}, err
	}
	fc, err := c.FileCabinet(ctx, cabinet)
	if err != nil {
		return nil, docuware.FileCabinet{}, docuware.Dialog{}, err
	}
	dlg, err := c.SearchDialog(ctx, fc, dialog)
	return c, fc, dlg, err
}

func (a *app) fieldsCmd() *cobra.Command {
	var dialog string
	cmd := &cobra.Command{
		Use:   "fields <cabinet>",
		Short: "Show the searchable fields of a file cabinet",
		Long: `Show the fields of the cabinet's search dialog. Use the FIELD column in
search conditions; fields marked LIST have predefined values (see dw values).`,
		Args: exactArgs(1, "a file cabinet name or id"),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, fc, dlg, err := a.searchDialog(cmd, args[0], dialog)
			if err != nil {
				return err
			}
			type row struct {
				Name       string `json:"name"`
				Label      string `json:"label"`
				Type       string `json:"type"`
				Length     int    `json:"length,omitempty"`
				SelectList bool   `json:"select_list,omitempty"`
			}
			rows := make([]row, 0, len(dlg.Fields))
			for _, f := range dlg.Fields {
				rows = append(rows, row{Name: f.DBFieldName, Label: f.Label(), Type: f.DWFieldType, Length: max(f.Length, 0), SelectList: f.HasSelectList()})
			}
			if a.jsonOut {
				return a.printJSON(map[string]any{
					"cabinet": map[string]string{"id": fc.ID, "name": fc.Name},
					"dialog":  map[string]string{"id": dlg.ID, "name": dlg.DisplayName},
					"fields":  rows,
				})
			}
			fprintf(a.stdout, "%s - search dialog %q\n\n", fc.Name, dlg.DisplayName)
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				length := ""
				if r.Length > 0 {
					length = strconv.Itoa(r.Length)
				}
				table = append(table, []string{r.Name, r.Label, r.Type, length, yesNo(r.SelectList)})
			}
			a.table([]string{"FIELD", "LABEL", "TYPE", "LENGTH", "LIST"}, table)
			return nil
		},
	}
	cmd.Flags().StringVarP(&dialog, "dialog", "d", "", "search dialog name or id (default: the cabinet's default)")
	return cmd
}

func (a *app) valuesCmd() *cobra.Command {
	var dialog string
	cmd := &cobra.Command{
		Use:   "values <cabinet> <field>",
		Short: "List the predefined values of a field",
		Args:  exactArgs(2, "a file cabinet and a field name"),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, dlg, err := a.searchDialog(cmd, args[0], dialog)
			if err != nil {
				return err
			}
			f, err := docuware.FindField(dlg.Fields, args[1])
			if err != nil {
				return err
			}
			values, err := c.SelectList(cmd.Context(), f)
			if err != nil {
				return err
			}
			if a.jsonOut {
				return a.printJSON(map[string]any{"field": f.DBFieldName, "values": orEmpty(values)})
			}
			for _, v := range values {
				fprintf(a.stdout, "%s\n", docuware.FormatValue(v))
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&dialog, "dialog", "d", "", "search dialog name or id")
	return cmd
}

// orEmpty keeps JSON output an array even when there are no results.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
