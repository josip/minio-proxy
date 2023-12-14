package minioproxy

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

type mockMinioFile struct {
	ContentType   string
	ContentLength int64
	Data          []byte
}

func (file *mockMinioFile) ETag() string {
	hash := md5.New()
	hash.Write(file.Data)
	return `"` + hex.EncodeToString(hash.Sum(nil)) + `"`
}

type mockFilePart struct {
	PartNumber int
	Data       []byte
}

type mockMinioServer struct {
	AccessKeyID               string
	AccessKeySecret           string
	Files                     map[string]*mockMinioFile
	MultipartUploads          map[string]map[string]mockFilePart
	CompletedMultipartUploads []string

	server *httptest.Server
}

func newMockMinioServer(keyId, secret string) *mockMinioServer {
	minio := &mockMinioServer{
		AccessKeyID:      keyId,
		AccessKeySecret:  secret,
		Files:            make(map[string]*mockMinioFile),
		MultipartUploads: make(map[string]map[string]mockFilePart),
	}
	minio.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path
		q := r.URL.Query()

		// upload file
		if r.Method == http.MethodPut {
			data, _ := io.ReadAll(r.Body)
			defer r.Body.Close()

			if q.Has("uploadId") && q.Has("partNumber") {
				// save part
				uploadID := q.Get("uploadId")
				partNumber, _ := strconv.Atoi(q.Get("partNumber"))
				partID := uploadID + "-p" + q.Get("partNumber")
				minio.MultipartUploads[uploadID][partID] = mockFilePart{
					PartNumber: partNumber,
					Data:       data,
				}
				w.Header().Set("ETag", partID)
			} else {
				// save file
				minio.Files[id] = &mockMinioFile{
					ContentType:   r.Header.Get("Content-Type"),
					ContentLength: r.ContentLength,
					Data:          data,
				}

				w.Header().Set("ETag", minio.Files[id].ETag())
			}

			time.Sleep(time.Duration(rand.Intn(3)) * time.Second)
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodPost {
			// initiate upload
			if q.Has("uploads") {
				uploadID := hex.EncodeToString(genRandBytes(8))
				minio.MultipartUploads[uploadID] = make(map[string]mockFilePart)
				resp := initiateMultipartUploadResult{
					Bucket:   strings.Split(r.URL.Path, "/")[0],
					UploadID: uploadID,
				}
				respXml, _ := xml.Marshal(resp)
				w.Write(respXml)
				return
			}

			// complete upload
			if uploadID := q.Get("uploadId"); len(uploadID) != 0 {
				var reqData completeMultipartUpload
				xmlDecoder := xml.NewDecoder(r.Body)
				if err := xmlDecoder.Decode(&reqData); err != nil {
					writeError(w, http.StatusNotImplemented, fmt.Errorf("can't decode multipart complete xml: %w", err))
					return
				}
				var completeData []byte
				for _, partInfo := range reqData.Parts {
					completeData = append(completeData, minio.MultipartUploads[uploadID][string(partInfo.ETag)].Data...)
				}
				delete(minio.MultipartUploads, uploadID)

				minio.Files[id] = &mockMinioFile{
					ContentType:   r.Header.Get("Content-Type"),
					ContentLength: int64(len(completeData)),
					Data:          completeData,
				}
				minio.CompletedMultipartUploads = append(minio.CompletedMultipartUploads, uploadID)

				w.Header().Set("ETag", minio.Files[id].ETag())
				w.WriteHeader(http.StatusOK)
				return
			}

			writeError(w, http.StatusNotImplemented, errors.New("unsupported POST"))
			return
		}

		// get file
		if r.Method == http.MethodGet {
			id := r.URL.Path
			f, exists := minio.Files[id]
			if !exists {
				writeError(w, http.StatusNotFound, fmt.Errorf("not found: %s", id))
				return
			}

			w.Header().Set("Content-Type", f.ContentType)
			w.Header().Set("Content-Length", strconv.FormatInt(f.ContentLength, 10))
			w.Header().Set("ETag", f.ETag())
			_, err := w.Write(f.Data)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
			}
		}
	}))

	return minio
}

func newMockPair() (*mockMinioServer, *minioClient) {
	keyID := "access-key-id"
	secretKey := "access-key-secret"
	minio := newMockMinioServer(keyID, secretKey)
	client := newMinioClient(minio.server.URL, keyID, secretKey)

	return minio, client
}

func verifyFilesMatch(client *minioClient, bucket, filename, contentType string, data []byte, chunkSize int64) error {
	contentLength := int64(len(data))
	etag, err := client.Upload(
		bucket, filename,
		contentType, contentLength,
		chunkSize,
		bytes.NewReader(data),
	)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	if len(etag) == 0 {
		return errors.New("upload failed: no etag")
	}

	file, err := client.GetFile(bucket, filename)
	if err != nil {
		return fmt.Errorf("failed to get file: %w", err)
	}

	if file.ContentLength != contentLength {
		return fmt.Errorf("uploaded file has different size, expected %d but got %d", contentLength, file.ContentLength)
	}

	downloadedData, err := io.ReadAll(file.Data)
	if err != nil {
		return fmt.Errorf("failed to read downloaded file: %w", err)
	}

	if !bytes.Equal(data, downloadedData) {
		return errors.New("downloaded data is not same as uploaded, expected")
	}

	return nil
}

func TestUploadDownloadSimpleFile(t *testing.T) {
	_, client := newMockPair()

	bucket := "testbucket"
	filename := "hello.txt"
	content := "hello world"

	err := verifyFilesMatch(client, bucket, filename, "text/plain", []byte(content), -1)
	if err != nil {
		t.Error(err)
	}
}

func TestChunkedUpload(t *testing.T) {
	minio, client := newMockPair()
	bucket := "testbucket"
	filename := "rand.dat"
	// NOTE this creates a 15mb file in memory
	content := genRandBytes(minChunkedFileSize + 44)
	err := verifyFilesMatch(client, bucket, filename, "text/plain", content, 3*1024*1024)
	if err != nil {
		t.Error(err)
	}

	if len(minio.CompletedMultipartUploads) != 1 {
		t.Error("expected for upload to happen with multipart but it did not")
	}
}
