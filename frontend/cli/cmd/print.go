package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type FormatOptions struct {
	Output OutputFormat
}

type ResourceFormatter interface {
	Display(resources any, format OutputFormat) error
}

type DefaultResourceFormatter struct{}

func (f *DefaultResourceFormatter) Display(resources any, format OutputFormat) (err error) {
	// if len(resources) == 0 {
	// 	return nil
	// }

	var output []byte
	switch format {
	case OutputFormatJSON:
		output, err = json.MarshalIndent(resources, "", "  ")
		if err != nil {
			return err
		}
	case OutputFormatYAML:
		output, err = yaml.Marshal(resources)
		if err != nil {
			return err
		}
	default:
		output, err = json.MarshalIndent(resources, "", "  ")
		if err != nil {
			return err
		}
	}

	fmt.Println(string(output))
	return nil
}

func addFormatOptions(cmd *cobra.Command, options *FormatOptions) {
	cmd.Flags().VarP(&options.Output, "output", "o", "output format (json, yaml)")
}

type OutputFormat string

const (
	OutputFormatJSON OutputFormat = "json"
	OutputFormatYAML OutputFormat = "yaml"
)

func (e *OutputFormat) String() string {
	return string(*e)
}

func (e *OutputFormat) Set(v string) error {
	switch v {
	case "json", "yaml":
		*e = OutputFormat(v)
		return nil
	default:
		return errors.New(`must be one of "json" or "yaml"`)
	}
}

func (e *OutputFormat) Type() string {
	return "outputformat"
}

func PtrToString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func toPtr[T any](v T) *T {
	return &v
}
