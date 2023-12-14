package minioproxy

import (
	"encoding/json"
	"net/http"
)

type jsonData map[string]string

func writeJson(w http.ResponseWriter, statusCode int, data jsonData) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, statusCode int, err error) {
	writeJson(w, statusCode, jsonData{"error": err.Error()})
}
