package service

import "example.com/shop/domain"

type Finder interface {
	Find(id domain.ID) (*domain.User, error)
}

type Repository interface {
	Finder
	Save(user *domain.User) error
}

type MemoryRepository struct {
	Users map[domain.ID]*domain.User
}

func (m *MemoryRepository) Find(id domain.ID) (*domain.User, error) {
	return m.Users[id], nil
}

func (m *MemoryRepository) Save(user *domain.User) error {
	m.Users[user.ID] = user
	return nil
}
