package prompt

import (
	_ "embed"
)

//go:embed plan.md
var Plan string

//go:embed edit.md
var Edit string

//go:embed compaction.md
var Compaction string
