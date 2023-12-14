package minioproxy

import (
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

type uploadApi struct {
	app *App
}

func bindUploadApi(app *App) {
	api := uploadApi{app: app}
	api.app.router.Methods("PUT").Path("/files/{filename}").HandlerFunc(api.handleUpload)
}

func (api *uploadApi) handleUpload(w http.ResponseWriter, r *http.Request) {
	filename := mux.Vars(r)["filename"]
	log.Println("PUT /files/" + filename)

	contentType := r.Header.Get("Content-Type")
	if len(contentType) == 0 {
		contentType = "application/octet-stream"
	}

	contentLength := r.ContentLength + int64(ENC_META_SIZE)

	start := time.Now().UnixMilli()
	etag, err := api.app.client.Upload(api.app.bucketName, filename, contentType, contentLength, api.app.chunkSize, api.encryptStream(r.Body))
	uploadDuration := time.Now().UnixMilli() - start

	log.Println("upload", filename, "took", uploadDuration, "ms")

	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJson(w, http.StatusAccepted, jsonData{
		"id":   filename,
		"etag": string(etag),
	})
}

func (api *uploadApi) encryptStream(input io.Reader) io.Reader {
	return encryptStream(api.app.encKey, api.app.hmacKey, input)
}
