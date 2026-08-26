package main

import (
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
