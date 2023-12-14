package minioproxy

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"

	"github.com/josip/minioproxy/presign"
)

var errFileNotFound = errors.New("file not found")
var errAccessForbidden = errors.New("access forbidden")

type minioClient struct {
	endpoint string
	signer   *presign.Signer

	// for testing
	http *http.Client
}

type ETag string

type minioFile struct {
	ContentType   string
	ContentLength int64
	ETag          ETag

	Data io.ReadCloser
}

func newMinioClient(endpoint, accessKeyID, accessSecret string) *minioClient {
	return &minioClient{
		endpoint: endpoint,
		signer: &presign.Signer{
			AccessKeyID:     accessKeyID,
			AccessKeySecret: accessSecret,
			Endpoint:        endpoint,
		},
		http: http.DefaultClient,
	}
}

func (c *minioClient) GetFile(bucket, filename string) (*minioFile, error) {
	resp, err := http.Get(c.signer.Presign("GET", bucket, filename, "10m", nil))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, errFileNotFound
		case http.StatusForbidden:
			return nil, errAccessForbidden
		default:
			return nil, fmt.Errorf("failed to get file info, unknown resp %d", resp.StatusCode)
		}
	}

	return &minioFile{
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
		ETag:          ETag(resp.Header.Get("ETag")),

		Data: resp.Body,
	}, nil
}

func (c *minioClient) Upload(bucket, filename, contentType string, contentLength, chunkSize int64, input io.Reader) (ETag, error) {
	chunks := c.chunksForFile(contentLength, chunkSize)
	if chunks <= 1 {
		// NOTE if input is coming from encryptStream, data will be still written
		// to the request's body in blocks of ENC_BUFFER_SIZE
		return c.uploadCommon("", -1, bucket, filename, contentType, contentLength, input)
	}

	mu := multipartUpload{
		client:        c,
		Bucket:        bucket,
		Filename:      filename,
		ContentType:   contentType,
		ContentLength: contentLength,
		ChunkSize:     chunkSize,
		Chunks:        chunks,
	}
	return mu.Upload(input)
}

func (c *minioClient) chunksForFile(contentLength, chunkSize int64) int {
	if contentLength < minChunkedFileSize || chunkSize < 1 {
		return 1
	}

	return int(math.Max(math.Ceil(float64(contentLength)/float64(chunkSize)), 1))
}

func (c *minioClient) uploadCommon(uploadID string, part int, bucket, filename, contentType string, contentLength int64, input io.Reader) (ETag, error) {
	reqOpts := url.Values{}
	if len(uploadID) > 0 && part > 0 {
		reqOpts.Set("partNumber", strconv.Itoa(part))
		reqOpts.Set("uploadId", uploadID)
	}
	uploadUrl := c.signer.Presign(http.MethodPut, bucket, filename, "1m", reqOpts)
	req, err := http.NewRequest(http.MethodPut, uploadUrl, input)
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", contentType)
	req.ContentLength = contentLength

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	etag := resp.Header.Get("Etag")

	if resp.StatusCode == http.StatusOK {
		return ETag(etag), nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return "", fmt.Errorf("failed to upload file: %s", respBody)
}
