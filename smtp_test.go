package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"slices"
	"strings"
	"testing"
)

func TestSessionMailAndRcpt(t *testing.T) {
	s := &Session{store: newStore(), logger: slog.New(slog.DiscardHandler)}

	want := []string{"c@b.com", "d@b.com"}
	for _, createAddr := range want {
		s.store.Create(createAddr)
	}

	if err := s.Mail("a@b.com", nil); err != nil {
		t.Fatalf("Mail: %v", err)
	}
	if s.sender != "a@b.com" {
		t.Errorf("sender = %q, want %q", s.sender, "a@b.com")
	}

	for _, address := range want {
		if err := s.Rcpt(address, nil); err != nil {
			t.Fatalf("Rcpt: %v", err)
		}
	}
	if !slices.Equal(s.recipients, want) {
		t.Errorf("recipients = %v, want %v", s.recipients, want)
	}
}

func TestSessionReset(t *testing.T) {
	s := &Session{
		store:      newStore(),
		logger:     slog.New(slog.DiscardHandler),
		sender:     "a@b.com",
		recipients: []string{"c@d.com"},
	}

	s.Reset()

	if s.sender != "" {
		t.Errorf("sender = %q, want empty string", s.sender)
	}
	if s.recipients != nil {
		t.Errorf("recipients = %v, want nil", s.recipients)
	}
}

func TestSessionData(t *testing.T) {
	tests := []struct {
		name            string
		createAddresses []string
		sender          string
		recipients      []string
		rawMessage      string
		wantMailbox     map[string]int
		wantErr         error
		wantParseErr    bool
	}{
		{
			name:            "single recipient",
			createAddresses: []string{"a@b.com"},
			sender:          "c@d.com",
			recipients:      []string{"a@b.com"},
			rawMessage:      "From: c@d.com\r\nTo: a@b.com\r\nSubject: Hi\r\n\r\nBody text",
			wantErr:         nil,
			wantMailbox:     map[string]int{"a@b.com": 1},
		},
		{
			name:            "multiple recipients each get their own copy",
			createAddresses: []string{"a@b.com", "b@b.com"},
			sender:          "c@d.com",
			recipients:      []string{"a@b.com", "b@b.com"},
			rawMessage:      "From: c@d.com\r\nSubject: Hi\r\n\r\nBody text",
			wantErr:         nil,
			wantMailbox:     map[string]int{"a@b.com": 1, "b@b.com": 1},
		},
		{
			name:            "unknown recipient mailbox",
			createAddresses: []string{},
			sender:          "c@d.com",
			recipients:      []string{"missing@b.com"},
			rawMessage:      "Subject: Hi\r\n\r\nBody text",
			wantErr:         ErrMailboxNotFound,
			wantMailbox:     map[string]int{},
		},
		{
			name:            "malformed message",
			createAddresses: []string{"a@b.com"},
			sender:          "c@d.com",
			recipients:      []string{"a@b.com"},
			rawMessage:      "this message has no header and body split",
			wantErr:         nil,
			wantParseErr:    true,
			wantMailbox:     map[string]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newStore()
			for _, address := range tt.createAddresses {
				store.Create(address)
			}

			s := &Session{
				store:      store,
				logger:     slog.New(slog.DiscardHandler),
				sender:     tt.sender,
				recipients: tt.recipients,
			}

			err := s.Data(strings.NewReader(tt.rawMessage))
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got err %v, want %v", err, tt.wantErr)
				}
			case tt.wantParseErr:
				if err == nil {
					t.Fatalf("err = nil, want parse error")
				}
			default:
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
			}

			for wantAddr, wantCount := range tt.wantMailbox {
				mb, ok := store.mailboxes[wantAddr]
				gotCount := 0
				if ok {
					gotCount = len(mb.messages)
				}
				if gotCount != wantCount {
					t.Errorf("mailbox %q has %d messages, want %d", wantAddr, gotCount, wantCount)
				}
			}
		})
	}
}

func TestSMTPServer(t *testing.T) {
	store := newStore()
	createAddr := "a@b.com"
	store.Create(createAddr)

	server := NewSMTPServer("localhost", 0, store, slog.New(slog.DiscardHandler))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("tcp server err: %v", err)
	}
	addr := ln.Addr()

	go server.Serve(ln)
	t.Cleanup(func() {
		server.Close()
	})

	from := "c@d.com"
	to := "a@b.com"
	subject := "Hi"
	body := "Body text\r\n"
	rawMessage := fmt.Appendf(nil, "From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", from, to, subject, body)
	if err := smtp.SendMail(addr.String(), nil, from, []string{to}, rawMessage); err != nil {
		t.Fatalf("SendMail() err: %v", err)
	}

	mb, ok := store.mailboxes[to]
	if !ok {
		t.Fatalf("expected mailbox doesn't exist")
	}

	if len(mb.messages) != 1 {
		t.Fatalf("mailbox has %d messages, want %d", len(mb.messages), 1)
	}
	msg := mb.messages[0]
	if msg.Sender != from {
		t.Errorf("got from %q, want %q", msg.Sender, from)
	}
	if msg.Subject != subject {
		t.Errorf("got subject %q, want %q", msg.Subject, subject)
	}
	if msg.Body != body {
		t.Errorf("got body %q, want %q", msg.Body, body)
	}
}

func TestSMTPRecipientDoesNotExist(t *testing.T) {
	store := newStore()

	server := NewSMTPServer("localhost", 0, store, slog.New(slog.DiscardHandler))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("tcp server err: %v", err)
	}
	addr := ln.Addr()

	go server.Serve(ln)
	t.Cleanup(func() {
		server.Close()
	})

	from := "c@d.com"
	to := "a@b.com"
	rawMessage := []byte("From: c@b.com\r\nTo: a@b.com\r\nSubject: Hi\r\n\r\nBody")
	err = smtp.SendMail(addr.String(), nil, from, []string{to}, rawMessage)
	if err == nil {
		t.Fatalf("expected err, got %v", err)
	}
	if !strings.HasPrefix(err.Error(), "550") {
		t.Fatalf("got err = %v, want 550 error", err)
	}
}
