package terminal

type appState int

const (
	StateNormal appState = iota
	StateWaiting
	StateError
	StateHelp
)

type uiMode int

const (
	ModeInput uiMode = iota
	ModeScroll
)
