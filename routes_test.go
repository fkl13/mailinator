package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func doRequest(t *testing.T, app *application, method, target string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	rec := httptest.NewRecorder()

	app.routes().ServeHTTP(rec, req)
	return rec
}

func TestCreateMailHandler(t *testing.T) {
	path := "/mailboxes"
	app := &application{
		store:  newStore(),
		logger: slog.New(slog.DiscardHandler),
	}
	rec := doRequest(t, app, http.MethodPost, path, nil)
	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		t.Fatalf("got %d, want %d", res.StatusCode, http.StatusCreated)
	}

	if res.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("got Content-Type %q, want %q", res.Header.Get("Content-Type"), "application/json")
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("expected error to be nil, got %v", err)
	}

	var body struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("couldn't unmarshal JSON, got %v", err)
	}

	if body.Address == "" {
		t.Fatalf("expected non-empty string address, got %v", body.Address)
	}

	if !strings.HasSuffix(body.Address, "@mailinator.local") {
		t.Fatalf("got %q, want address with suffix %q", body.Address, "@mailinator.local")
	}

	if !app.store.Exists(body.Address) {
		t.Fatalf("expected mailbox %q to exist in store", body.Address)
	}
}

func TestCreateMessageHandler(t *testing.T) {
	tests := []struct {
		name           string
		createAddress  string
		lookupAddress  string
		message        string
		wantStatusCode int
		wantSender     string
		wantSubject    string
		wantBody       string
	}{
		{
			name:           "mailbox doesn't exist",
			createAddress:  "a@b.com",
			lookupAddress:  "b@b.com",
			message:        `{"message": {"sender": "c@c.com", "subject": "hi", "body": "body"}}`,
			wantStatusCode: http.StatusNotFound,
			wantSender:     "",
			wantSubject:    "",
			wantBody:       "",
		},
		{
			name:           "malformed JSON",
			createAddress:  "a@b.com",
			lookupAddress:  "a@b.com",
			message:        `{"message": {"sender": "c@c.com", "subject": "hi", "body": "body"`,
			wantStatusCode: http.StatusBadRequest,
			wantSender:     "",
			wantSubject:    "",
			wantBody:       "",
		},
		{
			name:           "valid message",
			createAddress:  "a@b.com",
			lookupAddress:  "a@b.com",
			message:        `{"message": {"sender": "c@c.com", "subject": "hi", "body": "body"}}`,
			wantStatusCode: http.StatusCreated,
			wantSender:     "c@c.com",
			wantSubject:    "hi",
			wantBody:       "body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &application{
				store:  newStore(),
				logger: slog.New(slog.DiscardHandler),
			}
			if ok := app.store.Create(tt.createAddress); !ok {
				t.Fatal("couldn't create mailbox")
			}

			path := fmt.Sprintf("/mailboxes/%s/messages", tt.lookupAddress)
			rec := doRequest(t, app, http.MethodPost, path, strings.NewReader(tt.message))
			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatusCode {
				t.Fatalf("got %d, want %d", res.StatusCode, tt.wantStatusCode)
			}

			if res.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("got Content-Type %q, want %q", res.Header.Get("Content-Type"), "application/json")
			}

			if tt.wantStatusCode != http.StatusCreated {
				return // error cases: status check is the assertion, nothing else to compare
			}

			data, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatalf("expected error to be nil, got %v", err)
			}

			var body struct {
				Message message `json:"message"`
			}
			if err := json.Unmarshal(data, &body); err != nil {
				t.Fatalf("couldn't unmarshal JSON, got %v", err)
			}

			got := body.Message
			if got.Sender != tt.wantSender || got.Subject != tt.wantSubject || got.Body != tt.wantBody {
				t.Errorf("got {%q %q %q}, want {%q %q %q}",
					got.Sender, got.Subject, got.Body,
					tt.wantSender, tt.wantSubject, tt.wantBody)
			}

			if got.ID == "" {
				t.Error("expected non-empty message ID")
			}
			if got.ReceivedAt.IsZero() {
				t.Error("expected ReceivedAt to be set")
			}
		})
	}
}

func newTestApp(t *testing.T, addresses []string) *application {
	app := &application{
		store:  newStore(),
		logger: slog.New(slog.DiscardHandler),
	}
	for _, address := range addresses {
		if ok := app.store.Create(address); !ok {
			t.Fatal("couldn't create mailbox")
		}
	}
	return app
}

func TestGetMessageHandler(t *testing.T) {
	tests := []struct {
		name            string
		createAddress   string
		lookupAddress   string
		lookupMessageID string
		sender          string
		subject         string
		body            string
		wantStatusCode  int
	}{
		{
			name:           "mailbox does not exist",
			createAddress:  "a@b.com",
			lookupAddress:  "b@b.com",
			wantStatusCode: http.StatusNotFound,
		},
		{
			name:            "message does not exist",
			createAddress:   "a@b.com",
			lookupAddress:   "a@b.com",
			lookupMessageID: "non-existing",
			wantStatusCode:  http.StatusNotFound,
		},
		{
			name:            "valid request",
			createAddress:   "a@b.com",
			lookupAddress:   "a@b.com",
			lookupMessageID: "",
			sender:          "c@c.com",
			subject:         "hi",
			body:            "body",
			wantStatusCode:  http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApp(t, []string{tt.createAddress})

			seedMessage, err := app.store.AddMessage(tt.createAddress, tt.sender, tt.subject, tt.body)
			if err != nil {
				t.Fatalf("couldn't create message: %v", err)
			}

			messageID := seedMessage.ID
			if tt.lookupMessageID != "" {
				messageID = tt.lookupMessageID
			}

			path := fmt.Sprintf("/mailboxes/%s/messages/%s", tt.lookupAddress, messageID)
			rec := doRequest(t, app, http.MethodGet, path, nil)
			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatusCode {
				t.Fatalf("got %d, want %d", res.StatusCode, tt.wantStatusCode)
			}

			if res.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("got Content-Type %q, want %q", res.Header.Get("Content-Type"), "application/json")
			}

			if tt.wantStatusCode != http.StatusOK {
				return // error cases: status check is the assertion, nothing else to compare
			}

			data, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatalf("expected error to be nil, got %v", err)
			}

			var body struct {
				Message message `json:"message"`
			}
			if err := json.Unmarshal(data, &body); err != nil {
				t.Fatalf("couldn't unmarshal JSON, got %v", err)
			}

			got := body.Message
			if got.Sender != tt.sender || got.Subject != tt.subject || got.Body != tt.body {
				t.Errorf("got {%q %q %q}, want {%q %q %q}",
					got.Sender, got.Subject, got.Body,
					tt.sender, tt.subject, tt.body)
			}

			if got.ID != seedMessage.ID {
				t.Errorf("got message id %s, want %s", got.ID, seedMessage.ID)
			}
			if got.ReceivedAt.IsZero() {
				t.Error("expected ReceivedAt to be set")
			}
		})
	}
}

type listMessageResponse struct {
	Messages   []listMessage `json:"messages"`
	NextCursor string        `json:"nextCursor"`
}

func TestListMessagesHandler(t *testing.T) {
	tests := []struct {
		name             string
		createAddress    string
		lookupAddress    string
		seedMessages     []seedMessage
		limit            string
		cursor           string
		wantNextCursor   bool
		wantStatusCode   int
		wantMessageCount int
	}{
		{
			name:             "mailbox does not exist",
			createAddress:    "a@b.com",
			lookupAddress:    "b@b.com",
			seedMessages:     []seedMessage{},
			limit:            strconv.Itoa(defaultLimit),
			cursor:           "",
			wantNextCursor:   false,
			wantStatusCode:   http.StatusNotFound,
			wantMessageCount: 0,
		},
		{
			name:             "limit equals 0",
			createAddress:    "a@b.com",
			lookupAddress:    "a@b.com",
			seedMessages:     []seedMessage{},
			limit:            "0",
			cursor:           "",
			wantNextCursor:   false,
			wantStatusCode:   http.StatusBadRequest,
			wantMessageCount: 0,
		},
		{
			name:             "limit is not a number",
			createAddress:    "a@b.com",
			lookupAddress:    "a@b.com",
			seedMessages:     []seedMessage{},
			limit:            "notanumber",
			cursor:           "",
			wantNextCursor:   false,
			wantStatusCode:   http.StatusBadRequest,
			wantMessageCount: 0,
		},
		{
			name:             "made up cursor",
			createAddress:    "a@b.com",
			lookupAddress:    "a@b.com",
			seedMessages:     []seedMessage{},
			limit:            strconv.Itoa(defaultLimit),
			cursor:           "made-up-cursor",
			wantNextCursor:   false,
			wantStatusCode:   http.StatusBadRequest,
			wantMessageCount: 0,
		},
		{
			name:          "valid request",
			createAddress: "a@b.com",
			lookupAddress: "a@b.com",
			seedMessages: []seedMessage{
				{sender: "c@c.com", subject: "one", body: "body 1"},
				{sender: "c@c.com", subject: "two", body: "body 2"},
			},
			limit:            "",
			cursor:           "",
			wantNextCursor:   false,
			wantStatusCode:   http.StatusOK,
			wantMessageCount: 2,
		},
		{
			name:          "valid request, response has nextCursor",
			createAddress: "a@b.com",
			lookupAddress: "a@b.com",
			seedMessages: []seedMessage{
				{sender: "c@c.com", subject: "one", body: "body 1"},
				{sender: "c@c.com", subject: "two", body: "body 2"},
				{sender: "c@c.com", subject: "three", body: "body 3"},
			},
			limit:            "2",
			cursor:           "",
			wantNextCursor:   true,
			wantStatusCode:   http.StatusOK,
			wantMessageCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApp(t, []string{tt.createAddress})

			seeded := []listMessage{}
			for _, msg := range tt.seedMessages {
				seedMessage, err := app.store.AddMessage(tt.createAddress, msg.sender, msg.subject, msg.body)
				if err != nil {
					t.Fatalf("couldn't create message: %v", err)
				}
				seeded = append(seeded, listMessage{
					ID:      seedMessage.ID,
					Sender:  seedMessage.Sender,
					Subject: seedMessage.Subject,
				})
			}
			slices.Reverse(seeded)

			path := listMessagesPath(tt.lookupAddress, tt.cursor, tt.limit)
			rec := doRequest(t, app, http.MethodGet, path, nil)
			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatusCode {
				t.Fatalf("got %d, want %d", res.StatusCode, tt.wantStatusCode)
			}

			if res.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("got Content-Type %q, want %q", res.Header.Get("Content-Type"), "application/json")
			}

			body := decodeJSONBody[listMessageResponse](t, res)

			if tt.wantStatusCode == http.StatusOK {
				if !tt.wantNextCursor && body.NextCursor != "" {
					t.Fatalf("want empty cursor, got %s", body.NextCursor)
				}
				if tt.wantNextCursor && body.NextCursor == "" {
					t.Fatalf("want cursor, got %q", body.NextCursor)
				}

				if !slices.Equal(body.Messages, seeded[:tt.wantMessageCount]) {
					t.Errorf("got messages = %+v, want %+v", body.Messages, seeded[:tt.wantMessageCount])
				}
			}
		})
	}
}

func TestListMessagesHandlerPagination(t *testing.T) {
	address := "a@b.com"
	seedMessages := []seedMessage{
		{sender: "c@c.com", subject: "one", body: "body 1"},
		{sender: "c@c.com", subject: "two", body: "body 2"},
		{sender: "c@c.com", subject: "three", body: "body 3"},
		{sender: "c@c.com", subject: "four", body: "body 4"},
	}
	limit := 2
	wantStatusCode := http.StatusOK
	app := newTestApp(t, []string{address})

	seeded := []listMessage{}
	for _, msg := range seedMessages {
		seedMessage, err := app.store.AddMessage(address, msg.sender, msg.subject, msg.body)
		if err != nil {
			t.Fatalf("couldn't create message: %v", err)
		}
		seeded = append(seeded, listMessage{
			ID:      seedMessage.ID,
			Sender:  seedMessage.Sender,
			Subject: seedMessage.Subject,
		})
	}
	slices.Reverse(seeded)

	path1 := listMessagesPath(address, "", strconv.Itoa(limit))
	rec1 := doRequest(t, app, http.MethodGet, path1, nil)
	res1 := rec1.Result()
	defer res1.Body.Close()

	if res1.StatusCode != wantStatusCode {
		t.Fatalf("got %d, want %d", res1.StatusCode, wantStatusCode)
	}

	page1body := decodeJSONBody[listMessageResponse](t, res1)

	if page1body.NextCursor == "" {
		t.Fatalf("want cursor to be not empty, got %s", page1body.NextCursor)
	}
	if !slices.Equal(page1body.Messages, seeded[:limit]) {
		t.Errorf("got messages = %+v, want %+v", page1body.Messages, seeded[:limit])
	}

	path2 := listMessagesPath(address, page1body.NextCursor, strconv.Itoa(limit))
	rec2 := doRequest(t, app, http.MethodGet, path2, nil)
	res2 := rec2.Result()
	defer res2.Body.Close()

	page2body := decodeJSONBody[listMessageResponse](t, res2)

	if page2body.NextCursor != "" {
		t.Fatalf("want cursor to be empty, got %s", page2body.NextCursor)
	}
	if !slices.Equal(page2body.Messages, seeded[limit:]) {
		t.Errorf("got messages = %+v, want %+v", page2body.Messages, seeded[limit:])
	}
}

func listMessagesPath(address, cursor, limit string) string {
	u := url.URL{Path: fmt.Sprintf("/mailboxes/%s/messages", address)}
	q := url.Values{}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if limit != "" {
		q.Set("limit", limit)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func decodeJSONBody[T any](t *testing.T, res *http.Response) T {
	t.Helper()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("expected error to be nil, got %v", err)
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("couldn't unmarshal JSON, got %v", err)
	}

	return v
}

func TestDeleteMailboxHandler(t *testing.T) {
	tests := []struct {
		name           string
		createAddress  string
		deleteAddress  string
		wantStatusCode int
	}{
		{
			name:           "mailbox does not exist",
			createAddress:  "a@b.com",
			deleteAddress:  "b@b.com",
			wantStatusCode: http.StatusNotFound,
		},
		{
			name:           "mailbox exists",
			createAddress:  "a@b.com",
			deleteAddress:  "a@b.com",
			wantStatusCode: http.StatusNoContent,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApp(t, []string{tt.createAddress})

			path := fmt.Sprintf("/mailboxes/%s", tt.deleteAddress)
			rec := doRequest(t, app, http.MethodDelete, path, nil)
			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatusCode {
				t.Fatalf("got status code %d, want %d", res.StatusCode, tt.wantStatusCode)
			}

			if tt.wantStatusCode != http.StatusNoContent {
				return
			}

			if app.store.Exists(tt.deleteAddress) {
				t.Fatalf("mailbox with address %q exists after delete", tt.deleteAddress)
			}
		})
	}
}

func TestDeleteMessageHandler(t *testing.T) {
	tests := []struct {
		name           string
		createAddress  string
		seedMessages   []seedMessage
		deleteAddress  string
		messageID      string
		messageIdx     int
		wantStatusCode int
	}{
		{
			name:           "mailbox does not exist",
			createAddress:  "a@b.com",
			deleteAddress:  "b@b.com",
			seedMessages:   []seedMessage{},
			messageID:      "not-relevant",
			messageIdx:     0,
			wantStatusCode: http.StatusNotFound,
		},
		{
			name:          "message does not exist",
			createAddress: "a@b.com",
			deleteAddress: "a@b.com",
			seedMessages: []seedMessage{
				{sender: "x@y.com", subject: "subject", body: "body"},
			},
			messageID:      "does_not_exist",
			messageIdx:     0,
			wantStatusCode: http.StatusNotFound,
		},
		{
			name:          "delete message",
			createAddress: "a@b.com",
			deleteAddress: "a@b.com",
			seedMessages: []seedMessage{
				{sender: "x@y.com", subject: "subject 1", body: "body 1"},
				{sender: "x@y.com", subject: "subject 2", body: "body 2"},
				{sender: "x@y.com", subject: "subject 3", body: "body 3"},
			},
			messageID:      "",
			messageIdx:     1,
			wantStatusCode: http.StatusNoContent,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApp(t, []string{tt.createAddress})

			seeded := []message{}
			for _, msg := range tt.seedMessages {
				seedMessage, err := app.store.AddMessage(tt.createAddress, msg.sender, msg.subject, msg.body)
				if err != nil {
					t.Fatalf("couldn't create message: %v", err)
				}
				seeded = append(seeded, seedMessage)
			}
			slices.Reverse(seeded)

			messageID := tt.messageID
			if messageID == "" {
				messageID = seeded[tt.messageIdx].ID
			}

			path := fmt.Sprintf("/mailboxes/%s/messages/%s", tt.deleteAddress, messageID)
			rec := doRequest(t, app, http.MethodDelete, path, nil)
			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatusCode {
				t.Fatalf("got status code %d, want %d", res.StatusCode, tt.wantStatusCode)
			}

			if tt.wantStatusCode != http.StatusNoContent {
				return
			}

			for _, msg := range app.store.mailboxes[tt.deleteAddress].messages {
				if msg.ID == messageID {
					t.Fatalf("expect message ID %s not in list, got %v", messageID, app.store.mailboxes[tt.deleteAddress].messages)
				}
			}
		})
	}
}
