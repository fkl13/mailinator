package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
	"sync"
	"time"
)

const (
	defaultLimit    = 10
	defaultMaxLimit = 50
)

var (
	ErrMailboxNotFound = errors.New("mailbox not found")
	ErrMessageNotFound = errors.New("message not found")
	ErrInvalidCursor   = errors.New("invalid cursor")
	ErrInvalidLimit    = errors.New("invalid limit")
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

func (s *store) ListMessages(address, cursor string, limit int) ([]message, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	mailbox, ok := s.mailboxes[address]
	if !ok {
		return []message{}, "", ErrMailboxNotFound
	}

	if limit <= 0 {
		return []message{}, "", ErrInvalidLimit
	}
	if limit > defaultMaxLimit {
		limit = defaultMaxLimit
	}

	endIdx := len(mailbox.messages)
	if cursor != "" {
		found := false
		for i := len(mailbox.messages) - 1; i > -1; i-- {
			if cursor == mailbox.messages[i].ID {
				endIdx = i
				found = true
				break
			}
		}

		if !found {
			return []message{}, "", ErrInvalidCursor
		}
	}

	startIdx := max(0, endIdx-limit)
	messages := make([]message, endIdx-startIdx)
	copy(messages, mailbox.messages[startIdx:endIdx])
	slices.Reverse(messages)

	nextCursor := ""
	if startIdx > 0 {
		last := messages[len(messages)-1]
		nextCursor = last.ID
	}

	return messages, nextCursor, nil
}

func generateID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}
