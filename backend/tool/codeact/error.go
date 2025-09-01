package codeact

import "github.com/furisto/construct/backend/tool/base"

type ErrorCode = base.ErrorCode

const (
	PathIsNotAbsolute  = base.PathIsNotAbsolute
	PathIsDirectory    = base.PathIsDirectory
	PathIsNotDirectory = base.PathIsNotDirectory
	PermissionDenied   = base.PermissionDenied
	FileNotFound       = base.FileNotFound
	DirectoryNotFound  = base.DirectoryNotFound
	CannotStatFile     = base.CannotStatFile
	GenericFileError   = base.GenericFileError
	Internal           = base.Internal
	None               = base.None
	InvalidArgument    = base.InvalidInput
)

const GenericSuggestion = base.GenericSuggestion

type ToolError = base.ToolError

var NewError = base.NewError
var NewCustomError = base.NewCustomError
