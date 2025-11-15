package filesystem

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/afero"

	"github.com/furisto/construct/backend/tool/base"
)

type ListFilesInput struct {
	Path      string
	Recursive bool
}

type ListFilesResult struct {
	Path    string           `json:"path"`
	Entries []DirectoryEntry `json:"entries"`
}

type DirectoryEntry struct {
	Name string `json:"n"`
	Type string `json:"t"`
	Size int64  `json:"s"`
}

func ListFiles(fsys afero.Fs, input *ListFilesInput) (*ListFilesResult, error) {
	if !filepath.IsAbs(input.Path) {
		return nil, base.NewError(base.PathIsNotAbsolute, "path", input.Path)
	}
	path := input.Path

	fileInfo, err := fsys.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("directory not found", "path", path)
			return nil, base.NewError(base.DirectoryNotFound, "path", path)
		}
		if os.IsPermission(err) {
			slog.Warn("permission denied listing directory", "path", path)
			return nil, base.NewError(base.PermissionDenied, "path", path)
		}
		slog.Error("failed to stat path", "path", path, "error", err)
		return nil, base.NewError(base.CannotStatFile, "path", path)
	}

	if !fileInfo.IsDir() {
		return nil, base.NewError(base.PathIsNotDirectory, "path", path)
	}

	entries := []DirectoryEntry{}
	if input.Recursive {
		err = afero.Walk(fsys, path, func(filePath string, entry fs.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if filePath == path {
				return nil
			}

			dirEntry, err := toDirectoryEntry(filePath, entry)
			if err != nil {
				return err
			}
			entries = append(entries, *dirEntry)
			return nil
		})

		if err != nil {
			if os.IsPermission(err) {
				slog.Warn("permission denied walking directory", "path", path)
				return nil, base.NewError(base.PermissionDenied, "path", path)
			}
			slog.Error("error walking directory", "path", path, "error", err)
			return nil, base.NewError(base.GenericFileError, "path", path, "error", err)
		}
	} else {
		dirEntries, err := afero.ReadDir(fsys, path)
		if err != nil {
			if os.IsPermission(err) {
				slog.Warn("permission denied reading directory", "path", path)
				return nil, base.NewError(base.PermissionDenied, "path", path)
			}
			slog.Error("error reading directory", "path", path, "error", err)
			return nil, base.NewError(base.GenericFileError, "path", path, "error", err)
		}

		for _, entry := range dirEntries {
			entryPath := filepath.Join(path, entry.Name())
			dirEntry, err := toDirectoryEntry(entryPath, entry)
			if err != nil {
				return nil, err
			}
			entries = append(entries, *dirEntry)
		}
	}

	slog.Debug("directory listing completed", "path", path, "recursive", input.Recursive, "entry_count", len(entries))

	return &ListFilesResult{
		Path:    path,
		Entries: entries,
	}, nil
}

func toDirectoryEntry(path string, info fs.FileInfo) (*DirectoryEntry, error) {
	if info.IsDir() {
		return &DirectoryEntry{
			Name: path,
			Type: "d",
			Size: 0,
		}, nil
	} else {
		return &DirectoryEntry{
			Name: path,
			Type: "f",
			Size: (info.Size() + 1023) / 1024, // Size in KB
		}, nil
	}
}
