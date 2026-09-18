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

// Errors returned by the store.
var (
	ErrMailboxNotFound = errors.New("mailbox not found") // The mailbox wasn't found in the store
	ErrMessageNotFound = errors.New("message not found") // The message inside a mailbox wasn't found
	ErrInvalidCursor   = errors.New("invalid cursor")    // The passed pagination cursor is invalid
	ErrInvalidLimit    = errors.New("invalid limit")     // The passed pagination limit is invalid
)

// A store stores mailboxes and their corresponding messages in a map in-memory.
type store struct {
	mailboxes map[string]*mailbox
	mu        sync.RWMutex
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

func newStore() *store {
	return &store{mailboxes: map[string]*mailbox{}}
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
	s.mu.RLock()
	defer s.mu.RUnlock()

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
	s.mu.RLock()
	defer s.mu.RUnlock()

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
	s.mu.RLock()
	defer s.mu.RUnlock()

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

func (s *store) DeleteMailbox(address string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.mailboxes[address]; !ok {
		return ErrMailboxNotFound
	}
	delete(s.mailboxes, address)

	return nil
}

func (s *store) DeleteMessage(address, messageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	mailbox, ok := s.mailboxes[address]
	if !ok {
		return ErrMailboxNotFound
	}

	for i, message := range mailbox.messages {
		if message.ID == messageID {
			mailbox.messages = slices.Delete(mailbox.messages, i, i+1)
			return nil
		}
	}

	return ErrMessageNotFound
}

func (s *store) Evict(mailboxTTL, messageTTL time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	evicted := 0
	now := time.Now()
	for address, mailbox := range s.mailboxes {
		before := len(mailbox.messages)
		if before == 0 && now.Sub(mailbox.createdAt) > mailboxTTL {
			delete(s.mailboxes, address)
			continue
		}
		mailbox.messages = slices.DeleteFunc(mailbox.messages, func(msg message) bool {
			return now.Sub(msg.ReceivedAt) > messageTTL
		})
		evicted += before - len(mailbox.messages)
	}
	return evicted
}

func generateID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}
