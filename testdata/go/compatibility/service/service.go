package service

import "example.com/shop/domain"

type BaseService struct {
	LoggerName string
}

type UserService struct {
	BaseService
	repository Repository
}

func (s *UserService) Find(id domain.ID) (*domain.User, error) {
	return s.repository.Find(id)
}

func (s *UserService) Save(user *domain.User) error {
	return s.repository.Save(user)
}
