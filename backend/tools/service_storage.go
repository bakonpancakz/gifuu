package tools

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

type StorageProvider interface {
	Start(stop context.Context, await *sync.WaitGroup) error
	Put(ctx context.Context, key, contentType string, r io.ReadSeeker) error
	Delete(ctx context.Context, keys ...string) error
}

var Storage StorageProvider

var (
	ErrStorageFileNotFound    = errors.New("file not found")
	ErrStorageInvalidFilename = errors.New("filename contains invalid characters")
)

func SetupStorage(stop context.Context, await *sync.WaitGroup) {
	t := time.Now()

	switch STORAGE_PROVIDER {
	case "s3":
		Storage = &storageProviderS3{}
	case "disk":
		Storage = &storageProviderDisk{}
	default:
		LoggerStorage.Log(FATAL, "Unknown Provider: %s", STORAGE_PROVIDER)
	}

	if err := Storage.Start(stop, await); err != nil {
		LoggerStorage.Log(FATAL, "Startup Failed: %s", err.Error())
	}
	LoggerStorage.Log(INFO, "Ready in %s", time.Since(t))
}
