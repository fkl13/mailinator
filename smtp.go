package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/mail"
	"time"

	"github.com/emersion/go-smtp"
)

func NewSMTPServer(domain string, port int, store *store, logger *slog.Logger) *smtp.Server {
	be := &SMTPBackend{
		store:  store,
		logger: logger,
	}
	server := smtp.NewServer(be)
	server.Addr = fmt.Sprintf(":%d", port)
	server.Domain = domain
	server.WriteTimeout = 30 * time.Second
	server.ReadTimeout = 30 * time.Second
	server.MaxMessageBytes = 1024 * 1024
	server.MaxRecipients = 50
	server.AllowInsecureAuth = true

	return server
}

type SMTPBackend struct {
	store  *store
	logger *slog.Logger
}

func (b *SMTPBackend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{
		store:  b.store,
		logger: b.logger,
	}, nil
}

type Session struct {
	store      *store
	logger     *slog.Logger
	sender     string
	recipients []string
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.sender = from
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	if !s.store.Exists(to) {
		return &smtp.SMTPError{Code: 550, Message: "no such mailbox"}
	}
	s.recipients = append(s.recipients, to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	mail, err := mail.ReadMessage(r)
	if err != nil {
		s.logger.Error("reading message", slog.String("error", err.Error()))
		return err
	}

	subject := mail.Header.Get("Subject")

	body, err := io.ReadAll(mail.Body)
	if err != nil {
		s.logger.Error("reading mail body", slog.String("error", err.Error()))
		return fmt.Errorf("reading mail body: %w", err)
	}

	for _, recipient := range s.recipients {
		_, err := s.store.AddMessage(recipient, s.sender, subject, string(body))
		if err != nil {
			s.logger.Error("storing mail", slog.String("error", err.Error()))
			return err
		}
	}

	return nil
}

func (s *Session) Logout() error {
	return nil
}

func (s *Session) Reset() {
	s.sender = ""
	s.recipients = nil
}
