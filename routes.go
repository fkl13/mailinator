package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

const maxMessageBytes = 1024 * 1024

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /mailboxes", app.createMailHandler)
	mux.HandleFunc("POST /mailboxes/{address}/messages", app.createMessageHandler)
	mux.HandleFunc("GET /mailboxes/{address}/messages/{id}", app.getMessageHandler)
	mux.HandleFunc("GET /mailboxes/{address}/messages", app.listMessagesHandler)
	mux.HandleFunc("DELETE /mailboxes/{address}", app.deleteMailboxHandler)
	mux.HandleFunc("DELETE /mailboxes/{address}/messages/{id}", app.deleteMessageHandler)

	return mux
}

func (app *application) createMailHandler(w http.ResponseWriter, r *http.Request) {
	var address string
	for {
		token, err := generateID()
		if err != nil {
			app.serverErrorResponse(w, err)
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
		app.serverErrorResponse(w, err)
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

	r.Body = http.MaxBytesReader(w, r.Body, maxMessageBytes)

	data, err := decodeJSON[CreateMessageRequest](r)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			app.errorResponse(w, http.StatusRequestEntityTooLarge, "Request body is too large")
			return
		}
		app.errorResponse(w, http.StatusBadRequest, "Failed to decode JSON")
		return
	}

	message, err := app.store.AddMessage(address, data.Message.Sender, data.Message.Subject, data.Message.Body)
	if errors.Is(err, ErrMailboxNotFound) {
		app.errorResponse(w, http.StatusNotFound, err.Error())
		return
	} else if err != nil {
		app.serverErrorResponse(w, err)
		return
	}

	envelope := envelope{"message": message}
	err = writeJSON(w, http.StatusCreated, envelope)
	if err != nil {
		app.serverErrorResponse(w, err)
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
		app.serverErrorResponse(w, err)
		return
	}
}

type listMessage struct {
	ID      string `json:"id"`
	Sender  string `json:"sender"`
	Subject string `json:"subject"`
}

func (app *application) listMessagesHandler(w http.ResponseWriter, r *http.Request) {
	address := r.PathValue("address")
	if address == "" {
		app.errorResponse(w, http.StatusBadRequest, "Address is missing")
		return
	}

	cursor := r.URL.Query().Get("cursor")
	limitStr := r.URL.Query().Get("limit")
	var limit int
	if limitStr == "" {
		limit = defaultLimit
	} else {
		parsed, err := strconv.ParseUint(limitStr, 10, 32)
		if err != nil {
			app.errorResponse(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = int(parsed)
	}

	messages, nextCursor, err := app.store.ListMessages(address, cursor, limit)
	if errors.Is(err, ErrMailboxNotFound) {
		app.errorResponse(w, http.StatusNotFound, err.Error())
		return
	} else if errors.Is(err, ErrInvalidCursor) {
		app.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	} else if errors.Is(err, ErrInvalidLimit) {
		app.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	} else if err != nil {
		app.serverErrorResponse(w, err)
		return
	}

	responseMessages := []listMessage{}
	for _, msg := range messages {
		responseMessages = append(responseMessages, listMessage{
			ID:      msg.ID,
			Sender:  msg.Sender,
			Subject: msg.Subject,
		})
	}

	envelope := envelope{
		"messages":   responseMessages,
		"nextCursor": nextCursor,
	}
	err = writeJSON(w, http.StatusOK, envelope)
	if err != nil {
		app.serverErrorResponse(w, err)
		return
	}
}

func (app *application) deleteMailboxHandler(w http.ResponseWriter, r *http.Request) {
	address := r.PathValue("address")
	if address == "" {
		app.errorResponse(w, http.StatusBadRequest, "Address is missing")
		return
	}

	err := app.store.DeleteMailbox(address)
	if err != nil {
		app.errorResponse(w, http.StatusNotFound, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) deleteMessageHandler(w http.ResponseWriter, r *http.Request) {
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

	err := app.store.DeleteMessage(address, messageID)
	if err != nil {
		app.errorResponse(w, http.StatusNotFound, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) errorResponse(w http.ResponseWriter, status int, message any) {
	data := envelope{"errors": message}
	err := writeJSON(w, status, data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (app *application) serverErrorResponse(w http.ResponseWriter, err error) {
	app.logger.Error("internal server error", "error", err)

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
