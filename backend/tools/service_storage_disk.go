package tools

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// NOTE:
// This provider was intended for local/development use therefore
// I have no idea if this is vulnerable to a path traversal attack

type storageProviderDisk struct {
	Base string
	Mode os.FileMode
}

func (o *storageProviderDisk) Start(stop context.Context, await *sync.WaitGroup) error {
	o.Base = STORAGE_DISK_DIRECTORY
	o.Mode = FILE_MODE
	return os.MkdirAll(o.Base, o.Mode)
}

func (o *storageProviderDisk) Put(ctx context.Context, key, contentType string, r io.ReadSeeker) error {
	pathname := filepath.Join(o.Base, filepath.Clean(key))

	if err := os.MkdirAll(filepath.Dir(pathname), o.Mode); err != nil {
		return err
	}

	f, err := os.OpenFile(pathname, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, o.Mode)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return err
	}

	return nil
}

func (o *storageProviderDisk) Delete(ctx context.Context, keys ...string) error {
	var errs []string
	for _, k := range keys {
		full := filepath.Join(o.Base, filepath.Clean(k))
		if err := os.Remove(full); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("fs errors:\n%s", strings.Join(errs, "\n"))
	}
	return nil
}
