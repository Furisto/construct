package model

// import "time"

type ModelProfile interface {
	Validate() error
	Kind() ProviderKind
}
