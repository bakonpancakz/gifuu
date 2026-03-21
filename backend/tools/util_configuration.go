package tools

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	EPOCH_MILLI        = 1207008000000      // Generic EPOCH (April 1st 2008, Teto b-day!)
	EPOCH_SECONDS      = EPOCH_MILLI / 1000 // Generic EPOCH in Seconds
	LIFETIME_POW_TOKEN = 5 * time.Minute    // Lifetime of a Proof of Work Token
	TIMEOUT_SHUTDOWN   = 1 * time.Minute    // Standard Timeout for Shutdowns
	TIMEOUT_CONTEXT    = 30 * time.Second   // Standard Timeout for Requests
	FILE_MODE          = os.FileMode(0770)
)

var (
	LIMIT_JSON             = EnvNumber("LIMIT_JSON", 8*1024)            // (  8KB) Size limit per incoming JSON string
	LIMIT_FILE             = EnvNumber("LIMIT_FILE", 100*1024*1024)     // (100MB) Size limit per incoming media file
	LIMIT_TEMP             = EnvNumber("LIMIT_TEMP", 16*1024*1024*1024) // ( 16GB) Disk space allowed for temporary files
	LIMIT_ENCODES          = EnvNumber("LIMIT_ENCODES", 1)              // Concurrent Uploads
	MACHINE_ID             = EnvString("MACHINE_ID", "0")
	MACHINE_HOSTNAME       = EnvString("MACHINE_HOSTNAME", "le fishe")
	MACHINE_PROVERB        = EnvString("MACHINE_PROVERB", "><> .o( blub blub)")
	STORAGE_PROVIDER       = EnvString("STORAGE_PROVIDER", "disk")
	STORAGE_DISK_TEMP      = EnvString("STORAGE_DISK_TEMP", "_temp")
	STORAGE_DISK_DIRECTORY = EnvString("STORAGE_DISK_DIRECTORY", "_public")
	STORAGE_S3_KEY_SECRET  = EnvString("STORAGE_S3_KEY_SECRET", "xyz")
	STORAGE_S3_KEY_ACCESS  = EnvString("STORAGE_S3_KEY_ACCESS", "123")
	STORAGE_S3_ENDPOINT    = EnvString("STORAGE_S3_ENDPOINT", "https://bucket.s3.region.host.tld")
	STORAGE_S3_REGION      = EnvString("STORAGE_S3_REGION", "region")
	STORAGE_S3_BUCKET      = EnvString("STORAGE_S3_BUCKET", "bucket")
	DATABASE_URL           = EnvString("DATABASE_URL", "postgresql://postgres:password@localhost:5432")
	ONNX_RUNTIME_PATH      = EnvString("ONNX_RUNTIME_PATH", "")
	ONNX_RUNTIME_CUDA      = EnvString("ONNX_RUNTIME_CUDA", "") != ""
	HTTP_ADDRESS           = EnvString("HTTP_ADDRESS", "127.0.0.1:8080")
	HTTP_IP_HEADERS        = EnvSlice("HTTP_IP_HEADERS", ",", []string{"X-Forwarded-For"})
	HTTP_IP_PROXIES        = EnvSlice("HTTP_IP_PROXIES", ",", []string{"127.0.0.1/8"})
	HTTP_DIFFICULTY        = EnvNumber("HTTP_DIFFICULTY", 18)
)

var (
	SYNC_UPLOADS atomic.Int64
	SYNC_ENCODES = NewSemaphore(LIMIT_ENCODES)
)

func init() {
	// Prepare Temp Directory
	if err := os.MkdirAll(STORAGE_DISK_TEMP, FILE_MODE); err != nil {
		LoggerInit.Log(FATAL, "Cannot Create Temp Directory")
	}

	// Check Executables
	if err := exec.Command("ffmpeg", "--help").Run(); err != nil {
		LoggerInit.Log(FATAL, "FFmpeg failed to start: %s", err)
	}
	if err := exec.Command("ffprobe", "--help").Run(); err != nil {
		LoggerInit.Log(FATAL, "FFprobe failed to start: %s", err)
	}

}

// Read String from Environment
func EnvString(field, initial string) string {
	if value := os.Getenv(field); value == "" {
		return initial
	} else {
		return value
	}
}

// Read String from Environment and Parse it as a slice using the given delimiter
func EnvSlice(field, delimiter string, initial []string) []string {
	if value := os.Getenv(field); value == "" {
		return initial
	} else {
		return strings.Split(value, delimiter)
	}
}

// Read String from Environment and Parse it as a number
func EnvNumber(field string, initial int) int {
	if value := os.Getenv(field); value == "" {
		return initial
	} else if number, err := strconv.Atoi(value); err != nil {
		return initial
	} else {
		return number
	}
}
