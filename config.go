package minioproxy

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const MIN_CHUNK_SIZE_MB = 5

// files need to be at least 15mb to use chunking
const minChunkedFileSize = 3 * MIN_CHUNK_SIZE_MB * 1024 * 1024
const maxChunkedFileSizeMB = 100

type Config struct {
	ServerAddr string
	Endpoint   string
	AccessKey  string
	SecretKey  string
	BucketName string
	// 0 to disable, has to be bigger than MIN_CHUNK_SIZE_MB
	UploadChunkSizeMb int

	EncKey  []byte
	HmacKey []byte
}

func (c *Config) uploadChunkSizeInBytes() int64 {
	return int64(c.UploadChunkSizeMb) * 1024 * 1024
}

func (c *Config) validate() error {
	var errs []error

	if len(c.ServerAddr) < 5 || !strings.Contains(c.ServerAddr, ":") {
		errs = append(errs, errors.New("invalid ServerAddr, :port required at least"))
	}
	if len(c.Endpoint) == 0 {
		errs = append(errs, errors.New("missing minio endpoint"))
	} else if _, err := url.Parse(c.Endpoint); err != nil {
		errs = append(errs, errors.New("endpoint is not a valid url"))
	}

	if len(c.AccessKey) == 0 {
		errs = append(errs, errors.New("missing minio access key"))
	}
	if len(c.SecretKey) == 0 {
		errs = append(errs, errors.New("missing minio secret key"))
	}
	if len(c.BucketName) == 0 {
		errs = append(errs, errors.New("missing BucketName"))
	}
	if len(c.EncKey) != 32 {
		errs = append(errs, errors.New("EncKey needs to be 32b"))
	}
	if len(c.HmacKey) != 32 {
		errs = append(errs, errors.New("HmacKey needs to be 32b"))
	}
	if bytes.Equal(c.EncKey, c.HmacKey) {
		errs = append(errs, errors.New("EncKey and HmacKey can't be same"))
	}
	if c.UploadChunkSizeMb > 0 && c.UploadChunkSizeMb < MIN_CHUNK_SIZE_MB {
		errs = append(errs, fmt.Errorf("UploadChunkSizeMb needs to be at least %d MB", MIN_CHUNK_SIZE_MB))
	}
	if c.UploadChunkSizeMb > maxChunkedFileSizeMB {
		errs = append(errs, fmt.Errorf("UploadChunkSizeMb can be max %d MB", maxChunkedFileSizeMB))
	}

	return errors.Join(errs...)
}
