// Package user provides user domain models and storage interfaces.
package user

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Role represents the user's role in the system.
type Role string

const (
	// RoleCustomer represents a customer who posts tenders.
	RoleCustomer Role = "customer"
	// RoleProvider represents a service provider who bids on tenders.
	RoleProvider Role = "provider"
	// RoleAdmin represents an administrator with full access.
	RoleAdmin Role = "admin"
	// RoleExecutor represents a user who can execute run commands.
	RoleExecutor Role = "executor"
)

// ValidRoles contains all valid role values.
var ValidRoles = []Role{RoleCustomer, RoleProvider, RoleAdmin, RoleExecutor}

// IsValid checks if the role is a valid role value.
func (r Role) IsValid() bool {
	for _, valid := range ValidRoles {
		if r == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the role.
func (r Role) String() string {
	return string(r)
}

// User represents a user in the system.
type User struct {
	// ID is the unique identifier for the user.
	ID string `json:"id"`
	// TelegramID is the user's Telegram user ID.
	TelegramID int64 `json:"telegram_id"`
	// Role is the user's role in the system.
	Role Role `json:"role"`
	// CurrentProjectID is the ID of the project the user is currently working with.
	// This is used for context tracking in commands and operations.
	// Empty string means no project is selected.
	CurrentProjectID string `json:"current_project_id,omitempty"`
}

// NewUser creates a new User with a generated UUID.
func NewUser(telegramID int64, role Role) (*User, error) {
	if !role.IsValid() {
		return nil, fmt.Errorf("invalid role: %s", role)
	}
	return &User{
		ID:         uuid.New().String(),
		TelegramID: telegramID,
		Role:       role,
	}, nil
}

// Validate checks if the user has valid field values.
func (u *User) Validate() error {
	if u.ID == "" {
		return errors.New("user ID is required")
	}
	if u.TelegramID == 0 {
		return errors.New("telegram ID is required")
	}
	if !u.Role.IsValid() {
		return fmt.Errorf("invalid role: %s", u.Role)
	}
	return nil
}

// CanExecute checks if the user has permission to execute run commands.
// Only users with 'executor' or 'admin' roles can execute run commands.
func (u *User) CanExecute() bool {
	return u.Role == RoleExecutor || u.Role == RoleAdmin
}

// CanDeploy checks if the user has permission to trigger deployments.
// Only users with 'admin' role can deploy - this is a privileged operation.
func (u *User) CanDeploy() bool {
	return u.Role == RoleAdmin
}

// SetCurrentProject sets the user's current project ID.
func (u *User) SetCurrentProject(projectID string) {
	u.CurrentProjectID = projectID
}

// ClearCurrentProject removes the user's current project selection.
func (u *User) ClearCurrentProject() {
	u.CurrentProjectID = ""
}

// HasCurrentProject checks if the user has a project currently selected.
func (u *User) HasCurrentProject() bool {
	return u.CurrentProjectID != ""
}
