package main

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

type createStep struct {
	address string
	wantOK  bool
}

func TestStoreCreateAndExists(t *testing.T) {
	tests := []struct {
		name  string
		steps []createStep
		check string
		want  bool
	}{
		{
			name: "exists after create",
			steps: []createStep{
				{address: "a@b.com", wantOK: true},
			},
			check: "a@b.com",
			want:  true,
		},
		{
			name: "unknown address",
			steps: []createStep{
				{address: "a@b.com", wantOK: true},
			},
			check: "b@b.com",
			want:  false,
		},
		{
			name:  "empty store",
			steps: []createStep{},
			check: "a@b.com",
			want:  false,
		},
		{
			name: "empty address string",
			steps: []createStep{
				{address: "a@b.com", wantOK: true},
			},
			check: "",
			want:  false,
		},
		{
			name: "many distinct addresses",
			steps: []createStep{
				{address: "a@b.com", wantOK: true},
				{address: "b@b.com", wantOK: true},
				{address: "c@b.com", wantOK: true},
			},
			check: "b@b.com",
			want:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			for i, step := range tt.steps {
				if got := s.Create(step.address); got != step.wantOK {
					t.Errorf("step %d: got %v for '%s', want %v", i, got, step.address, step.wantOK)
				}
			}

			got := s.Exists(tt.check)
			if got != tt.want {
				t.Errorf("Got %v for address '%s', want %v", got, tt.check, tt.want)
			}
		})
	}
}

func TestCreateDuplicate(t *testing.T) {
	tests := []struct {
		name  string
		steps []createStep
	}{
		{
			name: "create same address twice",
			steps: []createStep{
				{address: "a@b.com", wantOK: true},
				{address: "a@b.com", wantOK: false},
			},
		},
		{
			name: "create two distinct addresses",
			steps: []createStep{
				{address: "a@a.com", wantOK: true},
				{address: "b@b.com", wantOK: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			for i, step := range tt.steps {
				before := s.mailboxes[step.address]
				got := s.Create(step.address)
				if got != step.wantOK {
					t.Errorf("step %d: got %v for '%s', want %v", i, got, step.address, step.wantOK)
				}

				if !step.wantOK {
					after := s.mailboxes[step.address]
					if !after.createdAt.Equal(before.createdAt) {
						t.Errorf("step %d: createdAt changed on rejected Create for %v", i, step.address)
					}
				}

			}
		})
	}
}

type addMessageStep struct {
	address string
	sender  string
	subject string
	body    string
	wantErr error
}

func TestAddMessage(t *testing.T) {
	tests := []struct {
		name         string
		addresses    []string
		steps        []addMessageStep
		checkAddress string
		wantCount    int
	}{
		{
			name:      "Message persists in mailbox",
			addresses: []string{"a@b.com"},
			steps: []addMessageStep{
				{address: "a@b.com", sender: "x@y.com", subject: "hi", body: "body", wantErr: nil},
			},
			checkAddress: "a@b.com",
			wantCount:    1,
		},
		{
			name:      "Unknown address",
			addresses: []string{"a@b.com"},
			steps: []addMessageStep{
				{address: "unknown@b.com", sender: "x@y.com", subject: "hi", body: "body", wantErr: ErrMailboxNotFound},
			},
			checkAddress: "a@b.com",
			wantCount:    0,
		},
		{
			name:      "Independent mailboxes",
			addresses: []string{"a@b.com", "b@b.com"},
			steps: []addMessageStep{
				{address: "a@b.com", sender: "x@y.com", subject: "hi", body: "body", wantErr: nil},
			},
			checkAddress: "b@b.com",
			wantCount:    0,
		},
		{
			name:      "Multiple messages same mailbox",
			addresses: []string{"a@b.com"},
			steps: []addMessageStep{
				{address: "a@b.com", sender: "x@y.com", subject: "one", body: "body 1", wantErr: nil},
				{address: "a@b.com", sender: "x@y.com", subject: "two", body: "body 2", wantErr: nil},
			},
			checkAddress: "a@b.com",
			wantCount:    2,
		},
		{
			name:      "Empty sender/subject/body allowed",
			addresses: []string{"a@b.com"},
			steps: []addMessageStep{
				{address: "a@b.com", sender: "", subject: "", body: "", wantErr: nil},
			},
			checkAddress: "a@b.com",
			wantCount:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			for _, address := range tt.addresses {
				s.Create(address)
			}

			seenIDs := map[string]bool{}
			for i, step := range tt.steps {
				msg, err := s.AddMessage(step.address, step.sender, step.subject, step.body)
				if !errors.Is(err, step.wantErr) {
					t.Errorf("step %d: err = %v, want %v", i, err, step.wantErr)
					continue
				}

				if step.wantErr != nil {
					continue
				}

				if msg.Sender != step.sender || msg.Subject != step.subject || msg.Body != step.body {
					t.Errorf("step %d: got message %+v want sender=%q subject=%q body=%q",
						i, msg, step.sender, step.subject, step.body)
				}

				if msg.ID == "" {
					t.Errorf("step %d: message id is empty", i)
				}
				if seenIDs[msg.ID] {
					t.Errorf("step %d: duplicate message id %q", i, msg.ID)
				}

				seenIDs[msg.ID] = true
			}

			mb, ok := s.mailboxes[tt.checkAddress]
			gotCount := 0
			if ok {
				gotCount = len(mb.messages)
			}
			if gotCount != tt.wantCount {
				t.Errorf("mailbox %q has %d messages, want %d", tt.checkAddress, gotCount, tt.wantCount)
			}
		})
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	s := NewStore()

	var wg sync.WaitGroup
	const goroutines = 50
	const iterations = 100

	for i := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			address := fmt.Sprintf("user%d@m.com", id)
			for range iterations {
				s.Create(address)
				s.Exists(address)
			}
		}(i)
	}

	wg.Wait()
}

type seedMessage struct {
	sender  string
	subject string
	body    string
}

func TestGetMessage(t *testing.T) {
	tests := []struct {
		name            string
		createAddresses []string
		seedAddress     string
		seedMessages    []seedMessage
		lookupAddress   string
		lookupIndex     int
		wantErr         error
	}{
		{
			name:            "message exists in store",
			createAddresses: []string{"a@b.com"},
			seedAddress:     "a@b.com",
			seedMessages:    []seedMessage{{sender: "x@y.com", subject: "hi", body: "body"}},
			lookupAddress:   "a@b.com",
			lookupIndex:     0,
			wantErr:         nil,
		},
		{
			name:            "message doesn't exist in store",
			createAddresses: []string{"a@b.com"},
			seedAddress:     "a@b.com",
			seedMessages:    []seedMessage{{sender: "x@y.com", subject: "hi", body: "body"}},
			lookupAddress:   "a@b.com",
			lookupIndex:     -1,
			wantErr:         ErrMessageNotFound,
		},
		{
			name:            "mailbox doesn't exist",
			createAddresses: []string{"a@b.com"},
			seedAddress:     "a@b.com",
			lookupAddress:   "unknown@b.com",
			lookupIndex:     -1,
			wantErr:         ErrMailboxNotFound,
		},
		{
			name:            "find message in list with several messages",
			createAddresses: []string{"a@b.com"},
			seedAddress:     "a@b.com",
			seedMessages: []seedMessage{
				{sender: "x@y.com", subject: "one", body: "body 1"},
				{sender: "x@y.com", subject: "two", body: "body 2"},
				{sender: "x@y.com", subject: "three", body: "body 3"},
			},
			lookupAddress: "a@b.com",
			lookupIndex:   1,
			wantErr:       nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			for _, address := range tt.createAddresses {
				s.Create(address)
			}

			var seeded []message
			for _, sm := range tt.seedMessages {
				msg, err := s.AddMessage(tt.seedAddress, sm.sender, sm.subject, sm.body)
				if err != nil {
					t.Fatalf("setup: AddMessage failed: %v", err)
				}
				seeded = append(seeded, msg)
			}

			messageID := "nonexistent-id"
			if tt.lookupIndex >= 0 {
				messageID = seeded[tt.lookupIndex].ID
			}

			got, gotErr := s.GetMessage(tt.lookupAddress, messageID)
			if !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("err = %v, want %v", gotErr, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}

			want := seeded[tt.lookupIndex]
			if got != want {
				t.Errorf("got %+v, want %+v", got, want)
			}
		})
	}
}
