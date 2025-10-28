package secret

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

type FileProvider struct {
	basePath string
	fs       afero.Fs
}

func NewFileProvider(basePath string, fs afero.Fs) (*FileProvider, error) {
	if err := fs.MkdirAll(basePath, 0700); err != nil {
		return nil, fmt.Errorf("failed to create secret directory: %w", err)
	}

	return &FileProvider{
		basePath: basePath,
		fs:       fs,
	}, nil
}

func (fp *FileProvider) Get(key string) (string, error) {
	filePath := filepath.Join(fp.basePath, key)

	data, err := afero.ReadFile(fp.fs, filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", &ErrSecretNotFound{Key: key, Err: err}
		}
		return "", fmt.Errorf("failed to read secret file: %w", err)
	}

	return string(data), nil
}

func (fp *FileProvider) Set(key string, value string) error {
	filePath := filepath.Join(fp.basePath, key)

	if err := afero.WriteFile(fp.fs, filePath, []byte(value), 0600); err != nil {
		return fmt.Errorf("failed to write secret file: %w", err)
	}

	return nil
}

func (fp *FileProvider) Delete(key string) error {
	filePath := filepath.Join(fp.basePath, key)

	if err := fp.fs.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete secret file: %w", err)
	}

	return nil
}
