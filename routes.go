package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mailboxes", app.createMailHandler)
	mux.HandleFunc("POST /mailboxes/{address}/messages", app.createMessageHandler)
	mux.HandleFunc("GET /mailboxes/{address}/messages/{id}", app.getMessageHandler)

	return mux
}

func (app *application) createMailHandler(w http.ResponseWriter, r *http.Request) {
	var address string
	for {
		token, err := generateID()
		if err != nil {
			app.serverErrorResponse(w)
			return
		}
		address = fmt.Sprintf("%s@mailinator.local", token)

		ok := app.store.Create(address)
		if ok {
			break
		}

	}

	data := envelope{"address": address}
	err := writeJSON(w, http.StatusCreated, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

type CreateMessageRequest struct {
	Message struct {
		Sender  string `json:"sender"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	} `json:"message"`
}

func (app *application) createMessageHandler(w http.ResponseWriter, r *http.Request) {
	address := r.PathValue("address")
	if address == "" {
		app.errorResponse(w, http.StatusBadRequest, "Address is missing")
		return
	}

	data, err := decodeJSON[CreateMessageRequest](r)
	if err != nil {
		app.errorResponse(w, http.StatusBadRequest, "Failed to decode JSON")
		return
	}

	message, err := app.store.AddMessage(address, data.Message.Sender, data.Message.Subject, data.Message.Body)
	if errors.Is(err, ErrMailboxNotFound) {
		app.errorResponse(w, http.StatusNotFound, err.Error())
		return
	} else if err != nil {
		app.serverErrorResponse(w)
		return
	}

	envelope := envelope{"message": message}
	err = writeJSON(w, http.StatusCreated, envelope)
	if err != nil {
		app.serverErrorResponse(w)
	}
}

func (app *application) getMessageHandler(w http.ResponseWriter, r *http.Request) {
	address := r.PathValue("address")
	if address == "" {
		app.errorResponse(w, http.StatusBadRequest, "Address is missing")
		return
	}

	messageID := r.PathValue("id")
	if messageID == "" {
		app.errorResponse(w, http.StatusBadRequest, "Message id is missing")
		return
	}

	message, err := app.store.GetMessage(address, messageID)
	if err != nil {
		app.errorResponse(w, http.StatusNotFound, err.Error())
		return
	}

	envelope := envelope{"message": message}
	err = writeJSON(w, http.StatusOK, envelope)
	if err != nil {
		app.serverErrorResponse(w)
		return
	}
}

func (app *application) errorResponse(w http.ResponseWriter, status int, message any) {
	data := envelope{"errors": message}
	err := writeJSON(w, status, data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (app *application) serverErrorResponse(w http.ResponseWriter) {
	message := "The server could not process your request"
	app.errorResponse(w, http.StatusInternalServerError, message)
}

type envelope map[string]any

func decodeJSON[T any](r *http.Request) (T, error) {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var v T
	if err := decoder.Decode(&v); err != nil {
		return v, err
	}
	return v, nil
}

func writeJSON(w http.ResponseWriter, status int, data envelope) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(encoded)

	return nil
}
