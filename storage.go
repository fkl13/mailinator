package main

import (
	"sync"
	"time"
)

type store struct {
	mailboxes map[string]mailbox
	mu        sync.Mutex
}

type mailbox struct {
	address   string
	createdAt time.Time
}

func NewStore() store {
	return store{mailboxes: map[string]mailbox{}}
}

func (s *store) Create(address string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.mailboxes[address]; ok {
		return false
	}

	mailbox := mailbox{
		address:   address,
		createdAt: time.Now(),
	}
	s.mailboxes[address] = mailbox
	return true
}

func (s *store) Exists(address string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.mailboxes[address]
	return ok
}
