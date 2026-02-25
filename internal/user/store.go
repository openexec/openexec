package user

import (
	"context"
	"errors"
)

// Common errors returned by UserStore implementations.
var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrInvalidUser       = errors.New("invalid user data")
)

// Store defines the interface for user persistence operations.
type Store interface {
	// Create stores a new user. Returns ErrUserAlreadyExists if a user
	// with the same ID or TelegramID already exists.
	Create(ctx context.Context, user *User) error

	// GetByID retrieves a user by their ID.
	// Returns ErrUserNotFound if the user doesn't exist.
	GetByID(ctx context.Context, id string) (*User, error)

	// GetByTelegramID retrieves a user by their Telegram ID.
	// Returns ErrUserNotFound if the user doesn't exist.
	GetByTelegramID(ctx context.Context, telegramID int64) (*User, error)

	// Update modifies an existing user.
	// Returns ErrUserNotFound if the user doesn't exist.
	Update(ctx context.Context, user *User) error

	// Delete removes a user by their ID.
	// Returns ErrUserNotFound if the user doesn't exist.
	Delete(ctx context.Context, id string) error

	// List returns all users. Returns an empty slice if no users exist.
	List(ctx context.Context) ([]*User, error)
}
