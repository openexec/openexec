package user

import (
	"context"
	"sync"
)

// MockStore is an in-memory implementation of Store for testing and development.
type MockStore struct {
	mu            sync.RWMutex
	users         map[string]*User // keyed by ID
	telegramIndex map[int64]string // TelegramID -> ID mapping
}

// NewMockStore creates a new MockStore instance.
func NewMockStore() *MockStore {
	return &MockStore{
		users:         make(map[string]*User),
		telegramIndex: make(map[int64]string),
	}
}

// Create stores a new user in memory.
func (m *MockStore) Create(ctx context.Context, user *User) error {
	if err := user.Validate(); err != nil {
		return ErrInvalidUser
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for existing user by ID
	if _, exists := m.users[user.ID]; exists {
		return ErrUserAlreadyExists
	}

	// Check for existing user by TelegramID
	if _, exists := m.telegramIndex[user.TelegramID]; exists {
		return ErrUserAlreadyExists
	}

	// Store a copy to prevent external modifications
	stored := &User{
		ID:               user.ID,
		TelegramID:       user.TelegramID,
		Role:             user.Role,
		CurrentProjectID: user.CurrentProjectID,
	}
	m.users[user.ID] = stored
	m.telegramIndex[user.TelegramID] = user.ID

	return nil
}

// GetByID retrieves a user by their ID.
func (m *MockStore) GetByID(ctx context.Context, id string) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	user, exists := m.users[id]
	if !exists {
		return nil, ErrUserNotFound
	}

	// Return a copy to prevent external modifications
	return &User{
		ID:               user.ID,
		TelegramID:       user.TelegramID,
		Role:             user.Role,
		CurrentProjectID: user.CurrentProjectID,
	}, nil
}

// GetByTelegramID retrieves a user by their Telegram ID.
func (m *MockStore) GetByTelegramID(ctx context.Context, telegramID int64) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, exists := m.telegramIndex[telegramID]
	if !exists {
		return nil, ErrUserNotFound
	}

	user := m.users[id]
	// Return a copy to prevent external modifications
	return &User{
		ID:               user.ID,
		TelegramID:       user.TelegramID,
		Role:             user.Role,
		CurrentProjectID: user.CurrentProjectID,
	}, nil
}

// Update modifies an existing user.
func (m *MockStore) Update(ctx context.Context, user *User) error {
	if err := user.Validate(); err != nil {
		return ErrInvalidUser
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.users[user.ID]
	if !exists {
		return ErrUserNotFound
	}

	// If TelegramID changed, check it's not taken by another user
	if existing.TelegramID != user.TelegramID {
		if otherID, taken := m.telegramIndex[user.TelegramID]; taken && otherID != user.ID {
			return ErrUserAlreadyExists
		}
		// Update the index
		delete(m.telegramIndex, existing.TelegramID)
		m.telegramIndex[user.TelegramID] = user.ID
	}

	// Update the stored user
	m.users[user.ID] = &User{
		ID:               user.ID,
		TelegramID:       user.TelegramID,
		Role:             user.Role,
		CurrentProjectID: user.CurrentProjectID,
	}

	return nil
}

// Delete removes a user by their ID.
func (m *MockStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	user, exists := m.users[id]
	if !exists {
		return ErrUserNotFound
	}

	delete(m.telegramIndex, user.TelegramID)
	delete(m.users, id)

	return nil
}

// List returns all users.
func (m *MockStore) List(ctx context.Context) ([]*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	users := make([]*User, 0, len(m.users))
	for _, user := range m.users {
		// Return copies to prevent external modifications
		users = append(users, &User{
			ID:               user.ID,
			TelegramID:       user.TelegramID,
			Role:             user.Role,
			CurrentProjectID: user.CurrentProjectID,
		})
	}

	return users, nil
}

// Ensure MockStore implements Store interface.
var _ Store = (*MockStore)(nil)
