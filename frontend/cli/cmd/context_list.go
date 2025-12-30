package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type contextListOptions struct {
	RenderOptions RenderOptions
}

func NewContextListCmd() *cobra.Command {
	var options contextListOptions

	cmd := &cobra.Command{
		Use:     "list [flags]",
		Short:   "List all configured contexts",
		Aliases: []string{"ls"},
		Example: `  # List all contexts
  construct context list

  # List contexts in JSON format
  construct context list --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			contextManager := getContextManager(cmd.Context())

			endpointContexts, err := contextManager.LoadContext()
			if err != nil {
				return fmt.Errorf("failed to load contexts: %w", err)
			}

			displays := make([]*ContextDisplay, 0, len(endpointContexts.Contexts))
			for name, context := range endpointContexts.Contexts {
				authStatus := "none"
				if context.Auth != nil && context.Auth.IsConfigured() {
					if context.Auth.Token != "" {
						authStatus = "token (inline)"
					} else if context.Auth.TokenRef != "" {
						authStatus = context.Auth.TokenRef
					}
				}

				displays = append(displays, &ContextDisplay{
					Name:     name,
					Endpoint: context.Address,
					Kind:     context.Kind,
					Auth:     authStatus,
					Current:  name == endpointContexts.CurrentContext,
				})
			}

			sort.Slice(displays, func(i, j int) bool {
				return displays[i].Name < displays[j].Name
			})

			return getRenderer(cmd.Context()).Render(displays, &options.RenderOptions)
		},
	}

	addRenderOptions(cmd, &options.RenderOptions)
	return cmd
}
