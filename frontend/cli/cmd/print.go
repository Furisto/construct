package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"text/tabwriter"

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
	OutputFormatJSON  OutputFormat = "json"
	OutputFormatYAML  OutputFormat = "yaml"
	OutputFormatTable OutputFormat = "table"
	OutputFormatCard  OutputFormat = "card"
)

func (e *OutputFormat) String() string {
	if e == nil {
		return ""
	}
	return string(*e)
}

func (e *OutputFormat) Set(v string) error {
	switch v {
	case "json", "yaml", "table", "card":
		*e = OutputFormat(v)
		return nil
	default:
		return errors.New(`must be one of "json" or "yaml"`)
	}
}

func (e *OutputFormat) Type() string {
	return "outputformat"
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

	cmd.Flags().VarP(&options.Format, "output", "o", fmt.Sprintf("output format (json, yaml, table, card)(default: %s)", options.Format))
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

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer tw.Flush()

	var headers []string
	for i := 0; i < itemType.NumField(); i++ {
		field := itemType.Field(i)
		if includeField(field, options.Wide) {
			headers = append(headers, field.Name)
		}
	}

	// print headers
	for i, header := range headers {
		if i > 0 {
			fmt.Fprint(tw, "\t")
		}
		fmt.Fprint(tw, header)
	}
	fmt.Fprintln(tw)

	// print values for each item
	for _, item := range items {
		if item.Kind() == reflect.Ptr {
			if item.IsNil() {
				continue
			}
			item = item.Elem()
		}

		for i, header := range headers {
			if i > 0 {
				fmt.Fprint(tw, "\t")
			}

			fieldValue := item.FieldByName(header)
			if !fieldValue.IsValid() {
				fmt.Fprint(tw, "")
				continue
			}

			switch fieldValue.Kind() {
			case reflect.Ptr:
				if fieldValue.IsNil() {
					fmt.Fprint(tw, "")
				} else {
					fmt.Fprint(tw, fieldValue.Elem().Interface())
				}
			case reflect.String:
				fmt.Fprint(tw, fieldValue.String())
			default:
				fmt.Fprint(tw, fieldValue.Interface())
			}
		}
		fmt.Fprintln(tw)
	}

	return nil
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

func includeField(field reflect.StructField, wide bool) bool {
	return field.IsExported() && (field.Tag.Get("detail") == "default" || (wide && field.Tag.Get("detail") == "full"))
}

func PtrToString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
