package prompt

import (
	_ "embed"
)

//go:embed plan.md
var Plan string

//go:embed edit.md
var Edit string

//go:embed scout.md
var Scout string
