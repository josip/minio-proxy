package minioproxy

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

type readApi struct {
	app *App
}

func bindReadApi(app *App) {
	api := readApi{app: app}
	api.app.router.Methods("GET").Path("/files/{filename}").HandlerFunc(api.handleRead)
}

func (api *readApi) handleRead(w http.ResponseWriter, r *http.Request) {
	filename := mux.Vars(r)["filename"]
	log.Println("GET /files/" + filename)

	file, err := api.app.client.GetFile(api.app.bucketName, filename)
	if err != nil || file.ContentLength == 0 {
		if errors.Is(err, errFileNotFound) {
			writeError(w, http.StatusNotFound, err)
		} else if errors.Is(err, errAccessForbidden) {
			writeError(w, http.StatusForbidden, err)
		} else {
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}
	defer file.Data.Close()

	encryptedSize := file.ContentLength
	clearSize := strconv.FormatInt(encryptedSize-int64(ENC_META_SIZE), 10)
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Length", clearSize)
	w.Header().Set("ETag", string(file.ETag))

	if err := api.decryptStream(file.Data, encryptedSize, w); err != nil {
		w.Header().Del("Content-Length")
		writeError(w, http.StatusInternalServerError, err)
	}
}

func (api *readApi) decryptStream(input io.Reader, fileSize int64, dest io.Writer) error {
	return decryptStream(api.app.encKey, api.app.hmacKey, input, fileSize, dest)
}
