package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/furisto/construct/frontend/cli/pkg/terminal"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type RenderOptions struct {
	Format    OutputFormat
	Wide      bool
	NoHeaders bool
	Columns   map[string]struct{}
}

type OutputFormat string

const (
	OutputFormatJSON     OutputFormat = "json"
	OutputFormatYAML     OutputFormat = "yaml"
	OutputFormatTable    OutputFormat = "table"
	OutputFormatCard     OutputFormat = "card"
	OutputFormatMarkdown OutputFormat = "markdown"
)

func (e *OutputFormat) String() string {
	if e == nil {
		return ""
	}
	return string(*e)
}

func (e *OutputFormat) Set(v string) error {
	if v == "md" {
		v = "markdown"
	}
	switch v {
	case "json", "yaml", "table", "card", "markdown":
		*e = OutputFormat(v)
		return nil
	default:
		return errors.New(`must be one of "json", "yaml", "table", "card", or "markdown"`)
	}
}

func (e *OutputFormat) Type() string {
	return "format"
}

func WithCardFormat(options *RenderOptions) *RenderOptions {
	options.Format = OutputFormatCard
	return options
}

func WithTableFormat(options *RenderOptions) {
	options.Format = OutputFormatTable
}

func addRenderOptions(cmd *cobra.Command, options *RenderOptions) {
	if options.Format == "" {
		WithTableFormat(options)
	}

	cmd.Flags().VarP(&options.Format, "output", "o", fmt.Sprintf("output format (json, yaml, table, card, markdown) (default: %s)", options.Format))
	cmd.Flags().BoolVarP(&options.Wide, "wide", "w", false, "output verbosity (default: false)")
	cmd.Flags().BoolVarP(&options.NoHeaders, "no-headers", "", false, "do not print headers (default: false)")
}

type OutputRenderer interface {
	Render(resources any, options *RenderOptions) error
}

type DefaultRenderer struct{}

var _ OutputRenderer = (*DefaultRenderer)(nil)

func (f *DefaultRenderer) Render(resources any, options *RenderOptions) (err error) {
	var output []byte
	switch options.Format {
	case OutputFormatJSON:
		output, err = json.MarshalIndent(resources, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(output))
	case OutputFormatYAML:
		output, err = yaml.Marshal(resources)
		if err != nil {
			return err
		}
		fmt.Println(string(output))
	case OutputFormatCard:
		err = renderCard(resources, options)
		if err != nil {
			return err
		}
	case OutputFormatTable:
		err = renderTable(resources, options)
		if err != nil {
			return err
		}
	case OutputFormatMarkdown:
		err = renderMarkdown(resources, options)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported output format: %s", options.Format)
	}

	return nil
}

func renderTable(resources any, options *RenderOptions) error {
	if resources == nil {
		return nil
	}

	value := reflect.ValueOf(resources)
	typ := reflect.TypeOf(resources)

	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
		typ = typ.Elem()
	}

	var items []reflect.Value
	var itemType reflect.Type

	// handle slice or single item
	if value.Kind() == reflect.Slice {
		if value.Len() == 0 {
			return nil
		}
		for i := 0; i < value.Len(); i++ {
			items = append(items, value.Index(i))
		}
		itemType = typ.Elem()
	} else {
		items = append(items, value)
		itemType = typ
	}

	if itemType.Kind() == reflect.Ptr {
		itemType = itemType.Elem()
	}

	if itemType.Kind() != reflect.Struct {
		return fmt.Errorf("displayTable only supports struct types, got %v", itemType.Kind())
	}

	// Collect headers
	var headers []string
	for i := 0; i < itemType.NumField(); i++ {
		field := itemType.Field(i)
		if includeField(field, options.Wide) {
			headers = append(headers, field.Name)
		}
	}

	if len(headers) == 0 {
		return nil
	}

	// First pass: collect all data and calculate column widths
	var rows [][]string
	widths := make([]int, len(headers))

	headerRow := make([]string, len(headers))
	for i, h := range headers {
		headerRow[i] = terminal.Bold(h)
		widths[i] = len(h) // Width without ANSI codes
	}
	rows = append(rows, headerRow)

	// Add data rows and update widths
	for _, item := range items {
		if item.Kind() == reflect.Ptr {
			if item.IsNil() {
				continue
			}
			item = item.Elem()
		}

		row := make([]string, len(headers))
		for i, header := range headers {
			fieldValue := item.FieldByName(header)
			var strVal string

			if !fieldValue.IsValid() {
				strVal = ""
			} else {
				switch fieldValue.Kind() {
				case reflect.Ptr:
					if fieldValue.IsNil() {
						strVal = ""
					} else {
						strVal = fmt.Sprint(fieldValue.Elem().Interface())
					}
				case reflect.String:
					strVal = fieldValue.String()
				default:
					strVal = fmt.Sprint(fieldValue.Interface())
				}
			}

			row[i] = strVal
			if len(strVal) > widths[i] {
				widths[i] = len(strVal)
			}
		}
		rows = append(rows, row)
	}

	// Second pass: print with proper alignment
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				fmt.Print("  ")
			}
			fmt.Print(cell)

			visualLen := len(stripANSI(cell))
			padding := widths[i] - visualLen
			if padding > 0 {
				fmt.Print(strings.Repeat(" ", padding))
			}
		}
		fmt.Println()
	}

	return nil
}

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}

func renderCard(resources any, options *RenderOptions) error {
	if resources == nil {
		return nil
	}

	value := reflect.ValueOf(resources)
	typ := reflect.TypeOf(resources)

	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
		typ = typ.Elem()
	}

	var items []reflect.Value
	var itemType reflect.Type

	// handle slice or single item
	if value.Kind() == reflect.Slice {
		if value.Len() == 0 {
			return nil
		}
		for i := 0; i < value.Len(); i++ {
			items = append(items, value.Index(i))
		}
		itemType = typ.Elem()
	} else {
		items = append(items, value)
		itemType = typ
	}

	if itemType.Kind() == reflect.Ptr {
		itemType = itemType.Elem()
	}

	if itemType.Kind() != reflect.Struct {
		return fmt.Errorf("displayCard only supports struct types, got %v", itemType.Kind())
	}

	// get field names and max width for alignment
	var fields []reflect.StructField
	maxFieldNameWidth := 0
	for i := 0; i < itemType.NumField(); i++ {
		field := itemType.Field(i)
		if includeField(field, options.Wide) {
			fields = append(fields, field)
			if len(field.Name) > maxFieldNameWidth {
				maxFieldNameWidth = len(field.Name)
			}
		}
	}

	// print each item as a card
	for idx, item := range items {
		if item.Kind() == reflect.Ptr {
			if item.IsNil() {
				continue
			}
			item = item.Elem()
		}

		// print each field as "FieldName: FieldValue"
		for _, field := range fields {
			fieldValue := item.FieldByName(field.Name)
			if !fieldValue.IsValid() {
				continue
			}

			var valueStr string
			switch fieldValue.Kind() {
			case reflect.Ptr:
				if fieldValue.IsNil() {
					valueStr = ""
				} else {
					valueStr = fmt.Sprint(fieldValue.Elem().Interface())
				}
			case reflect.String:
				valueStr = fieldValue.String()
			default:
				valueStr = fmt.Sprint(fieldValue.Interface())
			}

			fmt.Printf("%-*s %s\n", maxFieldNameWidth+1, field.Name+":", valueStr)
		}

		// add blank line between cards (except after the last one)
		if idx < len(items)-1 {
			fmt.Println()
		}
	}

	return nil
}

func renderMarkdown(resources any, options *RenderOptions) error {
	if resources == nil {
		return nil
	}

	value := reflect.ValueOf(resources)
	typ := reflect.TypeOf(resources)

	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
		typ = typ.Elem()
	}

	var items []reflect.Value
	var itemType reflect.Type

	if value.Kind() == reflect.Slice {
		if value.Len() == 0 {
			return nil
		}
		for i := 0; i < value.Len(); i++ {
			items = append(items, value.Index(i))
		}
		itemType = typ.Elem()
	} else {
		items = append(items, value)
		itemType = typ
	}

	if itemType.Kind() == reflect.Ptr {
		itemType = itemType.Elem()
	}

	if itemType.Kind() != reflect.Struct {
		return fmt.Errorf("renderMarkdown only supports struct types, got %v", itemType.Kind())
	}

	var fields []reflect.StructField
	for i := 0; i < itemType.NumField(); i++ {
		field := itemType.Field(i)
		if includeField(field, options.Wide) {
			fields = append(fields, field)
		}
	}

	if len(fields) == 0 {
		return nil
	}

	for idx, item := range items {
		if item.Kind() == reflect.Ptr {
			if item.IsNil() {
				continue
			}
			item = item.Elem()
		}

		for _, field := range fields {
			fieldValue := item.FieldByName(field.Name)
			if !fieldValue.IsValid() {
				continue
			}

			valueStr := formatMarkdownValue(fieldValue)
			fmt.Printf("**%s:** %s\n", field.Name, valueStr)
		}

		if idx < len(items)-1 {
			fmt.Print("\n---\n\n")
		}
	}

	return nil
}

func formatMarkdownValue(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}

	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			return ""
		}
		return formatMarkdownValue(v.Elem())
	case reflect.String:
		return v.String()
	case reflect.Slice:
		if v.Len() == 0 {
			return ""
		}
		var parts []string
		for i := 0; i < v.Len(); i++ {
			parts = append(parts, formatMarkdownValue(v.Index(i)))
		}
		return strings.Join(parts, ", ")
	case reflect.Map:
		if v.Len() == 0 {
			return ""
		}
		var parts []string
		iter := v.MapRange()
		for iter.Next() {
			k := fmt.Sprint(iter.Key().Interface())
			val := formatMarkdownValue(iter.Value())
			parts = append(parts, fmt.Sprintf("%s=%s", k, val))
		}
		return strings.Join(parts, ", ")
	case reflect.Struct:
		if v.Type().String() == "time.Time" {
			return fmt.Sprint(v.Interface())
		}
		return fmt.Sprint(v.Interface())
	default:
		return fmt.Sprint(v.Interface())
	}
}

func includeField(field reflect.StructField, wide bool) bool {
	return field.IsExported() && (field.Tag.Get("detail") == "default" || (wide && field.Tag.Get("detail") == "full"))
}

func PtrToString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
