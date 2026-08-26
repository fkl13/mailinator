package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
)

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mailboxes", app.createMailHandler)

	return mux
}

func (app *application) createMailHandler(w http.ResponseWriter, r *http.Request) {
	var address string
	for {
		buf := make([]byte, 8)
		if _, err := rand.Read(buf); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		address = fmt.Sprintf("%s@mailinator.local", hex.EncodeToString(buf))

		ok := app.store.Create(address)
		if ok {
			break
		}

	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"address": address})
}
