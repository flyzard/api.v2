package domain

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

type User struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
}

func NewUser(email, name string) (User, error) {
	email = strings.TrimSpace(email)
	name = strings.TrimSpace(name)
	if email == "" {
		return User{}, errors.New("user email is required")
	}
	if len(email) > 254 {
		return User{}, errors.New("user email exceeds 254 chars")
	}
	if name == "" {
		return User{}, errors.New("user name is required")
	}
	return User{
		ID:    uuid.New(),
		Email: email,
		Name:  name,
	}, nil
}
