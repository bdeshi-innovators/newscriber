package users

import (
	"context"
	"sync"
	"time"
)

type InMemoryUserRepository struct {
	mu    sync.Mutex
	users map[string]*User
}

func NewInMemoryUserRepository() *InMemoryUserRepository {
	return &InMemoryUserRepository{users: make(map[string]*User)}
}

func (r *InMemoryUserRepository) GetUser(_ context.Context, phone string) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.users[phone]; ok {
		clone := *u
		return &clone, nil
	}
	return nil, nil
}

func (r *InMemoryUserRepository) UpsertUser(_ context.Context, phone, lang string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.users[phone]; ok {
		existing.LanguagePref = lang
		return nil
	}
	r.users[phone] = &User{
		PhoneNumber:  phone,
		LanguagePref: lang,
		CreatedAt:    time.Now().UTC(),
	}
	return nil
}
