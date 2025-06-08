module github.com/furisto/construct/backend

go 1.24.1

require (
	connectrpc.com/connect v1.18.1
	entgo.io/ent v0.14.4
	github.com/anthropics/anthropic-sdk-go v0.2.0-alpha.13
	github.com/cohesion-org/deepseek-go v1.2.7
	github.com/furisto/construct/api/go v0.0.0-00010101000000-000000000000
	github.com/google/go-cmp v0.6.0
	github.com/google/uuid v1.6.0
	github.com/googleapis/go-type-adapters v1.0.1
	github.com/grafana/sobek v0.0.0-20250320150027-203dc85b6d98
	github.com/invopop/jsonschema v0.13.0
	github.com/mattn/go-sqlite3 v1.14.27
	github.com/maypok86/otter v1.2.4
	github.com/openai/openai-go v1.2.0
	github.com/shopspring/decimal v1.4.0
	github.com/spf13/afero v1.14.0
	github.com/tink-crypto/tink-go v0.0.0-20230613075026-d6de17e3f164
	github.com/zalando/go-keyring v0.2.6
	google.golang.org/genproto v0.0.0-20240311173647-c811ad7063a7
	google.golang.org/protobuf v1.36.6
	k8s.io/client-go v0.32.3
)

require (
	al.essio.dev/pkg/shellescape v1.5.1 // indirect
	ariga.io/atlas v0.31.1-0.20250212144724-069be8033e83 // indirect
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.6-20250307204501-0409229c3780.1 // indirect
	github.com/agext/levenshtein v1.2.1 // indirect
	github.com/apparentlymart/go-textseg/v13 v13.0.0 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/bmatcuk/doublestar v1.3.4 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/danieljoos/wincred v1.2.2 // indirect
	github.com/dlclark/regexp2 v1.11.4 // indirect
	github.com/dolthub/maphash v0.1.0 // indirect
	github.com/gammazero/deque v1.0.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-openapi/inflect v0.19.0 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/google/pprof v0.0.0-20241029153458-d1b30febd7db // indirect
	github.com/hashicorp/hcl/v2 v2.13.0 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mitchellh/go-wordwrap v0.0.0-20150314170334-ad45545899c7 // indirect
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	github.com/zclconf/go-cty v1.14.4 // indirect
	github.com/zclconf/go-cty-yaml v1.1.0 // indirect
	go.uber.org/mock v0.5.2 // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/mod v0.23.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	golang.org/x/time v0.7.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apimachinery v0.32.3 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/utils v0.0.0-20241104100929-3ea5e8cea738 // indirect
)

replace github.com/furisto/construct/api/go => ../api/go
