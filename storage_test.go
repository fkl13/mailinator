package main

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"
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
			s := newStore()
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
			s := newStore()
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
			s := newStore()
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
	s := newStore()

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
			s := newStore()
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

func TestListMessages(t *testing.T) {
	tests := []struct {
		name           string
		seedAddress    string
		seedMessages   []seedMessage
		lookupAddress  string
		limit          int
		cursor         string
		wantNextCursor string
		wantErr        error
	}{
		{
			name:          "Empty mailbox",
			seedAddress:   "a@b.com",
			seedMessages:  []seedMessage{},
			lookupAddress: "a@b.com",
			limit:         defaultLimit,
			cursor:        "",
			wantErr:       nil,
		},
		{
			name:          "Unknown address",
			seedAddress:   "a@b.com",
			seedMessages:  []seedMessage{},
			lookupAddress: "b@b.com",
			limit:         defaultLimit,
			cursor:        "",
			wantErr:       ErrMailboxNotFound,
		},
		{
			name:        "Known address, made-up cursor",
			seedAddress: "a@b.com",
			seedMessages: []seedMessage{
				{sender: "x@y.com", subject: "one", body: "body 1"},
				{sender: "x@y.com", subject: "two", body: "body 2"},
				{sender: "x@y.com", subject: "three", body: "body 3"},
			},
			lookupAddress: "a@b.com",
			limit:         defaultLimit,
			cursor:        "made-up cursor",
			wantErr:       ErrInvalidCursor,
		},
		{
			name:        "Invalid limit",
			seedAddress: "a@b.com",
			seedMessages: []seedMessage{
				{sender: "x@y.com", subject: "one", body: "body 1"},
			},
			lookupAddress: "a@b.com",
			limit:         0,
			cursor:        "",
			wantErr:       ErrInvalidLimit,
		},
		{
			name:        "Fewer messages than limit",
			seedAddress: "a@b.com",
			seedMessages: []seedMessage{
				{sender: "x@y.com", subject: "one", body: "body 1"},
				{sender: "x@y.com", subject: "two", body: "body 2"},
			},
			lookupAddress: "a@b.com",
			limit:         defaultLimit,
			cursor:        "",
			wantErr:       nil,
		},
		{
			name:        "Messages exactly equal to limit",
			seedAddress: "a@b.com",
			seedMessages: []seedMessage{
				{sender: "x@y.com", subject: "one", body: "body 1"},
				{sender: "x@y.com", subject: "two", body: "body 2"},
			},
			lookupAddress: "a@b.com",
			limit:         2,
			cursor:        "",
			wantErr:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore()
			s.Create(tt.seedAddress)

			seeded := []message{}
			for _, sm := range tt.seedMessages {
				msg, err := s.AddMessage(tt.seedAddress, sm.sender, sm.subject, sm.body)
				if err != nil {
					t.Fatalf("setup: AddMessage failed: %v", err)
				}
				seeded = append(seeded, msg)
			}

			slices.Reverse(seeded) // ListMessage returns in recency order

			gotMessages, gotCursor, gotErr := s.ListMessages(tt.lookupAddress, tt.cursor, tt.limit)
			if !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("err = %v, want %v", gotErr, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}

			if gotCursor != tt.wantNextCursor {
				t.Errorf("got cursor = %v, want %q", gotCursor, tt.cursor)
			}

			if !reflect.DeepEqual(gotMessages, seeded) {
				t.Errorf("got messages = %+v, want %+v", gotMessages, seeded)
			}
		})
	}
}

func TestListMessagesPagination(t *testing.T) {
	seedAddress := "a@b.com"
	seedMessages := []seedMessage{
		{sender: "x@y.com", subject: "one", body: "body 1"},
		{sender: "x@y.com", subject: "two", body: "body 2"},
		{sender: "x@y.com", subject: "three", body: "body 3"},
	}
	limit := 2

	s := newStore()
	s.Create(seedAddress)

	seeded := []message{}
	for _, sm := range seedMessages {
		msg, err := s.AddMessage(seedAddress, sm.sender, sm.subject, sm.body)
		if err != nil {
			t.Fatalf("setup: AddMessage failed: %v", err)
		}
		seeded = append(seeded, msg)
	}
	slices.Reverse(seeded)

	gotMessages, gotCursor, gotErr := s.ListMessages(seedAddress, "", limit)
	if gotErr != nil {
		t.Fatalf("got error %v, expected no error", gotErr)
	}
	if gotCursor == "" {
		t.Errorf("got cursor %v, want non-empty", gotCursor)
	}
	if !reflect.DeepEqual(gotMessages, seeded[:limit]) {
		t.Errorf("got messages = %+v, want %+v", gotMessages, seeded[:limit])
	}

	gotMessages2, gotCursor2, gotErr := s.ListMessages(seedAddress, gotCursor, limit)
	if gotErr != nil {
		t.Fatalf("got error %v, expected no error", gotErr)
	}
	if gotCursor2 != "" {
		t.Errorf("got cursor %v, want empty string", gotCursor2)
	}
	if !reflect.DeepEqual(gotMessages2, seeded[limit:]) {
		t.Errorf("got messages = %+v, want %+v", gotMessages2, seeded[limit:])
	}
}

func TestDeleteMailbox(t *testing.T) {
	tests := []struct {
		name          string
		seedAddress   string
		deleteAddress string
		wantErr       error
	}{
		{
			name:          "mailbox does not exist",
			seedAddress:   "a@b.com",
			deleteAddress: "b@b.com",
			wantErr:       ErrMailboxNotFound,
		},
		{
			name:          "delete mailbox successfully",
			seedAddress:   "a@b.com",
			deleteAddress: "a@b.com",
			wantErr:       nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore()
			s.Create(tt.seedAddress)

			gotErr := s.DeleteMailbox(tt.deleteAddress)
			if !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("got error: %v, want %v", gotErr, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}

			if _, ok := s.mailboxes[tt.deleteAddress]; ok {
				t.Errorf("address %q not remove from store: %v", tt.deleteAddress, s.mailboxes)
			}
		})
	}
}

func TestDeleteMessage(t *testing.T) {
	tests := []struct {
		name          string
		seedAddress   string
		seedMessages  []seedMessage
		deleteAddress string
		messageID     string
		messageIdx    int
		wantErr       error
	}{
		{
			name:          "mailbox does not exist",
			seedAddress:   "a@b.com",
			seedMessages:  []seedMessage{},
			deleteAddress: "b@b.com",
			messageID:     "not relevant",
			messageIdx:    0,
			wantErr:       ErrMailboxNotFound,
		},
		{
			name:        "message does not exist",
			seedAddress: "a@b.com",
			seedMessages: []seedMessage{
				{sender: "x@y.com", subject: "one", body: "body 1"},
				{sender: "x@y.com", subject: "two", body: "body 2"},
				{sender: "x@y.com", subject: "three", body: "body 3"},
			},
			deleteAddress: "a@b.com",
			messageID:     "does not exist",
			messageIdx:    0,
			wantErr:       ErrMessageNotFound,
		},
		{
			name:        "delete message in the middle of the list",
			seedAddress: "a@b.com",
			seedMessages: []seedMessage{
				{sender: "x@y.com", subject: "one", body: "body 1"},
				{sender: "x@y.com", subject: "two", body: "body 2"},
				{sender: "x@y.com", subject: "three", body: "body 3"},
			},
			deleteAddress: "a@b.com",
			messageID:     "",
			messageIdx:    1,
			wantErr:       nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore()
			s.Create(tt.seedAddress)

			seeded := []message{}
			for _, sm := range tt.seedMessages {
				msg, err := s.AddMessage(tt.seedAddress, sm.sender, sm.subject, sm.body)
				if err != nil {
					t.Fatalf("setup: AddMessage failed: %v", err)
				}
				seeded = append(seeded, msg)
			}

			messageID := tt.messageID
			if messageID == "" {
				messageID = seeded[tt.messageIdx].ID
			}

			gotErr := s.DeleteMessage(tt.deleteAddress, messageID)
			if !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("got error: %v, want %v", gotErr, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}

			for _, msg := range s.mailboxes[tt.deleteAddress].messages {
				if msg.ID == messageID {
					t.Fatalf("expect message ID %s not in list, got %v", messageID, s.mailboxes[tt.deleteAddress].messages)
				}
			}
		})
	}
}

type seedEvictMessage struct {
	seedMessage
	backdated   time.Duration
	wantEvicted bool
}

type seedEvictMailbox struct {
	address     string
	backdated   time.Duration
	wantEvicted bool
}

func TestEvict(t *testing.T) {
	tests := []struct {
		name         string
		seedMailbox  []seedEvictMailbox
		seedMessages []seedEvictMessage
		messageTTL   time.Duration
		mailboxTTL   time.Duration
		wantCount    int
	}{
		{
			name: "empty mailbox but too young to evict",
			seedMailbox: []seedEvictMailbox{
				{address: "a@a.com", backdated: 0 * time.Minute, wantEvicted: false},
				{address: "b@b.com", backdated: 0 * time.Minute, wantEvicted: false},
			},
			seedMessages: []seedEvictMessage{},
			messageTTL:   2 * time.Hour,
			mailboxTTL:   4 * time.Hour,
			wantCount:    0,
		},
		{
			name: "no eviction",
			seedMailbox: []seedEvictMailbox{
				{address: "a@a.com", backdated: 0 * time.Minute, wantEvicted: false},
				{address: "b@b.com", backdated: 0 * time.Minute, wantEvicted: false},
			},
			seedMessages: []seedEvictMessage{
				{sender: "x@y.com", subject: "one", body: "body 1", backdated: 0 * time.Minute, wantEvicted: false},
				{sender: "x@y.com", subject: "two", body: "body 2", backdated: 59 * time.Minute, wantEvicted: false},
				{sender: "x@y.com", subject: "three", body: "body 3", backdated: 0 * time.Minute, wantEvicted: false},
			},
			messageTTL: 1 * time.Hour,
			mailboxTTL: 4 * time.Hour,
			wantCount:  0,
		},
		{
			name: "message evictions",
			seedMailbox: []seedEvictMailbox{
				{address: "a@a.com", backdated: 0 * time.Minute, wantEvicted: false},
				{address: "b@b.com", backdated: 0 * time.Minute, wantEvicted: false},
			},
			seedMessages: []seedEvictMessage{
				{sender: "x@y.com", subject: "one", body: "body 1", backdated: 0 * time.Minute, wantEvicted: false},
				{sender: "x@y.com", subject: "two", body: "body 2", backdated: 2 * time.Hour, wantEvicted: true},
				{sender: "x@y.com", subject: "three", body: "body 3", backdated: 1*time.Hour + 1*time.Second, wantEvicted: true},
			},
			messageTTL: 1 * time.Hour,
			mailboxTTL: 4 * time.Hour,
			wantCount:  4,
		},
		{
			name: "evict mailboxes",
			seedMailbox: []seedEvictMailbox{
				{address: "a@a.com", backdated: 2 * time.Hour, wantEvicted: true},
				{address: "b@b.com", backdated: 2 * time.Hour, wantEvicted: true},
			},
			seedMessages: []seedEvictMessage{},
			messageTTL:   1 * time.Hour,
			mailboxTTL:   1 * time.Hour,
			wantCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore()
			seeded := map[string][]message{}
			for _, mb := range tt.seedMailbox {
				s.Create(mb.address)
				s.mailboxes[mb.address].createdAt = s.mailboxes[mb.address].createdAt.Add(-mb.backdated)

				for i, sm := range tt.seedMessages {
					msg, err := s.AddMessage(mb.address, sm.sender, sm.subject, sm.body)
					if err != nil {
						t.Fatalf("setup: AddMessage failed: %v", err)
					}

					msg.ReceivedAt = msg.ReceivedAt.Add(-sm.backdated)
					s.mailboxes[mb.address].messages[i] = msg
					if !sm.wantEvicted {
						seeded[mb.address] = append(seeded[mb.address], msg)
					}
				}
			}

			evicted := s.Evict(tt.mailboxTTL, tt.messageTTL)
			if evicted != tt.wantCount {
				t.Fatalf("got %d evicted, want %d", evicted, tt.wantCount)
			}

			for _, mb := range tt.seedMailbox {
				if mb.wantEvicted {
					if s.Exists(mb.address) {
						t.Errorf("mailbox %q still present, want eviction", mb.address)
					}
					continue
				}

				want := seeded[mb.address]
				got := s.mailboxes[mb.address].messages
				if len(want) == 0 && len(got) == 0 {
					continue
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("mailbox %q messages = %+v, want %+v", mb.address, got, want)
				}
			}
		})
	}
}
