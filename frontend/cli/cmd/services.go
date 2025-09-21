package cmd

import (
	"context"
	"net/http"

	api "github.com/furisto/construct/api/go/client"
	"github.com/furisto/construct/shared"
	"github.com/furisto/construct/shared/config"
	"github.com/spf13/afero"
)

type ContextKey string

const (
	ContextKeyAPIClient       ContextKey = "api_client"
	ContextKeyFileSystem      ContextKey = "filesystem"
	ContextKeyOutputRenderer  ContextKey = "output_renderer"
	ContextKeyCommandRunner   ContextKey = "command_runner"
	ContextKeyEndpointContext ContextKey = "endpoint_context"
	ContextKeyRuntimeInfo     ContextKey = "runtime_info"
	ContextKeyUserInfo        ContextKey = "user_info"
	ContextKeyGlobalOptions   ContextKey = "global_options"
	ContextKeyHttpClient      ContextKey = "http_client"
	ContextKeyConfigStore     ContextKey = "config_store"
)

func getAPIClient(ctx context.Context) *api.Client {
	apiClient := ctx.Value(ContextKeyAPIClient)
	if apiClient != nil {
		return apiClient.(*api.Client)
	}

	return nil
}

func getFileSystem(ctx context.Context) *afero.Afero {
	fs := ctx.Value(ContextKeyFileSystem)
	if fs != nil {
		return fs.(*afero.Afero)
	}

	return &afero.Afero{Fs: afero.NewOsFs()}
}

func getCommandRunner(ctx context.Context) shared.CommandRunner {
	runner := ctx.Value(ContextKeyCommandRunner)
	if runner != nil {
		return runner.(shared.CommandRunner)
	}

	return &shared.DefaultCommandRunner{}
}

func getRuntimeInfo(ctx context.Context) shared.RuntimeInfo {
	runtimeInfo := ctx.Value(ContextKeyRuntimeInfo)
	if runtimeInfo != nil {
		return runtimeInfo.(shared.RuntimeInfo)
	}

	return &shared.DefaultRuntimeInfo{}
}

func getUserInfo(ctx context.Context) shared.UserInfo {
	userInfo := ctx.Value(ContextKeyUserInfo)
	if userInfo != nil {
		return userInfo.(shared.UserInfo)
	}

	return shared.NewDefaultUserInfo(getFileSystem(ctx))
}

func getRenderer(ctx context.Context) OutputRenderer {
	printer := ctx.Value(ContextKeyOutputRenderer)
	if printer != nil {
		return printer.(OutputRenderer)
	}

	return &DefaultRenderer{}
}

func setGlobalOptions(ctx context.Context, options *globalOptions) context.Context {
	return context.WithValue(ctx, ContextKeyGlobalOptions, options)
}

func getGlobalOptions(ctx context.Context) *globalOptions {
	if opts, ok := ctx.Value(ContextKeyGlobalOptions).(*globalOptions); ok {
		return opts
	}
	return &globalOptions{}
}

func getHttpClient(ctx context.Context) *http.Client {
	if httpClient := ctx.Value(ContextKeyHttpClient); httpClient != nil {
		return httpClient.(*http.Client)
	}

	return http.DefaultClient
}

func getConfigStore(ctx context.Context) *config.Store {
	if configStore := ctx.Value(ContextKeyConfigStore); configStore != nil {
		return configStore.(*config.Store)
	}

	return nil
}

func setConfigStore(ctx context.Context, configStore *config.Store) context.Context {
	return context.WithValue(ctx, ContextKeyConfigStore, configStore)
}
