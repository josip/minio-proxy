package minioproxy

import (
	"bytes"
	"cmp"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"slices"
)

type chunk struct {
	Part int
	Data []byte
	Size int64
}

// chunks become completedParts after they are uploaded
type completedPart struct {
	PartNumber int
	ETag       ETag
}

type initiateMultipartUploadResult struct {
	Bucket   string
	UploadID string `xml:"UploadId"`
}

type completeMultipartUpload struct {
	XMLName xml.Name        `xml:"http://s3.amazonaws.com/doc/2006-03-01/ CompleteMultipartUpload" json:"-"`
	Parts   []completedPart `xml:"Part"`
}

type multipartUpload struct {
	Bucket        string
	Filename      string
	ContentType   string
	ContentLength int64
	ChunkSize     int64
	Chunks        int

	client   *minioClient
	uploadID string
}

var errUploadAlreadyStarted = errors.New("upload already started")

const maxWorkers = 4

func (m *multipartUpload) Upload(input io.Reader) (ETag, error) {
	if len(m.uploadID) != 0 {
		return "", errUploadAlreadyStarted
	}

	// in case we have <4 chunks we create one worker per chunk
	workers := int(math.Min(maxWorkers, float64(m.Chunks)))

	log.Printf("uploading %s of %d MB in %d chunks of %d MB with %d workers\n",
		m.Filename, m.ContentLength/1024/1024,
		m.Chunks, m.ChunkSize/1024/1024, workers,
	)

	jobs := make(chan chunk, workers)
	completedParts := make(chan completedPart, m.Chunks)

	uploadID, err := m.initiate()
	if err != nil {
		return "", errors.Join(errors.New("failed to initiate upload"), err)
	}
	m.uploadID = uploadID

	for i := 0; i < workers; i++ {
		go m.uploader(i, jobs, completedParts)
	}

	// TODO abort upload if it has not been completed
	go func() {
		for i := 0; i < m.Chunks; i++ {
			data := make([]byte, m.ChunkSize)
			// NOTE int64 -> int
			n, err := io.ReadAtLeast(input, data, int(m.ChunkSize))
			if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
				log.Println("error while reading encrypted input", err)
				close(jobs)
				return
			}

			if n > 0 {
				jobs <- chunk{i + 1, data[:n], int64(n)}
			}

			if err == io.EOF || err == io.ErrUnexpectedEOF {
				close(jobs)
				break
			}
		}
	}()

	allCompletedParts, err := m.collectCompletitions(completedParts)
	if err != nil {
		return "", err
	}

	return m.complete(allCompletedParts)
}

func (m *multipartUpload) initiate() (string, error) {
	reqParams := url.Values{}
	reqParams.Add("uploads", "")

	reqUrl := m.client.signer.Presign("POST", m.Bucket, m.Filename, "1m", reqParams)
	resp, err := m.client.http.Post(reqUrl, m.ContentType, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var respData initiateMultipartUploadResult
	xmlDecoder := xml.NewDecoder(resp.Body)
	if err := xmlDecoder.Decode(&respData); err != nil {
		return "", err
	}

	return respData.UploadID, nil
}

func (m *multipartUpload) uploader(id int, jobs <-chan chunk, results chan<- completedPart) {
	for job := range jobs {
		if len(job.Data) == 0 {
			log.Println("upload worker", id, "tried to process empty job")
			continue
		}

		etag, err := m.client.uploadCommon(m.uploadID, job.Part, m.Bucket, m.Filename, m.ContentType, job.Size, bytes.NewReader(job.Data))
		if err == nil {
			log.Println("upload worker", id, "chunk", job.Part, "✔︎")
		} else {
			log.Println("upload worker", id, "chunk", job.Part, "X", err)
		}

		results <- completedPart{job.Part, etag}
	}
}

func (m *multipartUpload) complete(completedParts []completedPart) (ETag, error) {
	reqOpts := url.Values{}
	reqOpts.Add("uploadId", m.uploadID)

	reqUrl := m.client.signer.Presign("POST", m.Bucket, m.Filename, "1m", reqOpts)
	slices.SortFunc(completedParts, func(a, b completedPart) int {
		return cmp.Compare(a.PartNumber, b.PartNumber)
	})

	body := completeMultipartUpload{Parts: completedParts}
	xmlBody, err := xml.Marshal(&body)

	if err != nil {
		return "", err
	}
	resp, err := m.client.http.Post(reqUrl, m.ContentType, bytes.NewReader(xmlBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", errors.New(string(respBody))
	}

	return ETag(resp.Header.Get("Etag")), nil
}

func (m *multipartUpload) collectCompletitions(parts <-chan completedPart) ([]completedPart, error) {
	all := make([]completedPart, m.Chunks)

	for i := 0; i < m.Chunks; i++ {
		part := <-parts
		if len(part.ETag) == 0 {
			return nil, fmt.Errorf("failed to upload chunk %d", part.PartNumber)
		}

		all[i] = part
	}

	return all, nil
}
