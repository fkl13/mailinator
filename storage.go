package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var (
	ErrMailboxNotFound = errors.New("mailbox not found")
	ErrMessageNotFound = errors.New("message not found")
)

type store struct {
	mailboxes map[string]*mailbox
	mu        sync.Mutex
}

type mailbox struct {
	address   string
	createdAt time.Time
	messages  []message
}

type message struct {
	ID         string    `json:"id"`
	Sender     string    `json:"sender"`
	Subject    string    `json:"subject"`
	Body       string    `json:"body"`
	ReceivedAt time.Time `json:"receivedAt"`
}

func NewStore() store {
	return store{mailboxes: map[string]*mailbox{}}
}

func (s *store) Create(address string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.mailboxes[address]; ok {
		return false
	}

	mb := &mailbox{
		address:   address,
		createdAt: time.Now(),
		messages:  []message{},
	}
	s.mailboxes[address] = mb
	return true
}

func (s *store) Exists(address string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.mailboxes[address]
	return ok
}

func (s *store) AddMessage(address, sender, subject, body string) (message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	mailbox, ok := s.mailboxes[address]
	if !ok {
		return message{}, ErrMailboxNotFound
	}

	id, err := generateID()
	if err != nil {
		return message{}, err
	}

	message := message{
		ID:         id,
		Sender:     sender,
		Subject:    subject,
		Body:       body,
		ReceivedAt: time.Now(),
	}
	mailbox.messages = append(mailbox.messages, message)

	return message, nil
}

func (s *store) GetMessage(address, messageID string) (message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	mailbox, ok := s.mailboxes[address]
	if !ok {
		return message{}, ErrMailboxNotFound
	}

	for _, message := range mailbox.messages {
		if message.ID == messageID {
			return message, nil
		}
	}

	return message{}, ErrMessageNotFound
}

func generateID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}
