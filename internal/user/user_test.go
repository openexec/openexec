package user

import (
	"testing"
)

func TestRoleIsValid(t *testing.T) {
	tests := []struct {
		name  string
		role  Role
		valid bool
	}{
		{"customer role is valid", RoleCustomer, true},
		{"provider role is valid", RoleProvider, true},
		{"admin role is valid", RoleAdmin, true},
		{"executor role is valid", RoleExecutor, true},
		{"empty role is invalid", Role(""), false},
		{"unknown role is invalid", Role("unknown"), false},
		{"uppercase role is invalid", Role("ADMIN"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.role.IsValid(); got != tt.valid {
				t.Errorf("Role(%q).IsValid() = %v, want %v", tt.role, got, tt.valid)
			}
		})
	}
}

func TestRoleString(t *testing.T) {
	tests := []struct {
		role     Role
		expected string
	}{
		{RoleCustomer, "customer"},
		{RoleProvider, "provider"},
		{RoleAdmin, "admin"},
		{RoleExecutor, "executor"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.role.String(); got != tt.expected {
				t.Errorf("Role.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNewUser(t *testing.T) {
	t.Run("creates valid user", func(t *testing.T) {
		user, err := NewUser(12345, RoleCustomer)
		if err != nil {
			t.Fatalf("NewUser() error = %v, want nil", err)
		}
		if user.ID == "" {
			t.Error("NewUser() ID is empty")
		}
		if user.TelegramID != 12345 {
			t.Errorf("NewUser() TelegramID = %v, want 12345", user.TelegramID)
		}
		if user.Role != RoleCustomer {
			t.Errorf("NewUser() Role = %v, want %v", user.Role, RoleCustomer)
		}
	})

	t.Run("rejects invalid role", func(t *testing.T) {
		_, err := NewUser(12345, Role("invalid"))
		if err == nil {
			t.Error("NewUser() with invalid role should return error")
		}
	})
}

func TestUserValidate(t *testing.T) {
	tests := []struct {
		name    string
		user    User
		wantErr bool
	}{
		{
			name: "valid user",
			user: User{
				ID:         "123e4567-e89b-12d3-a456-426614174000",
				TelegramID: 12345,
				Role:       RoleProvider,
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			user: User{
				ID:         "",
				TelegramID: 12345,
				Role:       RoleProvider,
			},
			wantErr: true,
		},
		{
			name: "missing TelegramID",
			user: User{
				ID:         "123e4567-e89b-12d3-a456-426614174000",
				TelegramID: 0,
				Role:       RoleProvider,
			},
			wantErr: true,
		},
		{
			name: "invalid role",
			user: User{
				ID:         "123e4567-e89b-12d3-a456-426614174000",
				TelegramID: 12345,
				Role:       Role("invalid"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.user.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("User.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUserCanExecute(t *testing.T) {
	tests := []struct {
		name       string
		role       Role
		canExecute bool
	}{
		{"admin can execute", RoleAdmin, true},
		{"executor can execute", RoleExecutor, true},
		{"customer cannot execute", RoleCustomer, false},
		{"provider cannot execute", RoleProvider, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{
				ID:         "test-id",
				TelegramID: 12345,
				Role:       tt.role,
			}
			if got := user.CanExecute(); got != tt.canExecute {
				t.Errorf("User.CanExecute() = %v, want %v", got, tt.canExecute)
			}
		})
	}
}

func TestUserCanDeploy(t *testing.T) {
	tests := []struct {
		name      string
		role      Role
		canDeploy bool
	}{
		{"admin can deploy", RoleAdmin, true},
		{"executor cannot deploy", RoleExecutor, false},
		{"customer cannot deploy", RoleCustomer, false},
		{"provider cannot deploy", RoleProvider, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{
				ID:         "test-id",
				TelegramID: 12345,
				Role:       tt.role,
			}
			if got := user.CanDeploy(); got != tt.canDeploy {
				t.Errorf("User.CanDeploy() = %v, want %v", got, tt.canDeploy)
			}
		})
	}
}

func TestUserCurrentProject(t *testing.T) {
	t.Run("new user has no current project", func(t *testing.T) {
		user, err := NewUser(12345, RoleExecutor)
		if err != nil {
			t.Fatalf("NewUser() error = %v", err)
		}
		if user.HasCurrentProject() {
			t.Error("New user should not have a current project")
		}
		if user.CurrentProjectID != "" {
			t.Errorf("New user CurrentProjectID = %q, want empty", user.CurrentProjectID)
		}
	})

	t.Run("set current project", func(t *testing.T) {
		user := &User{
			ID:         "test-id",
			TelegramID: 12345,
			Role:       RoleExecutor,
		}
		projectID := "project-123"
		user.SetCurrentProject(projectID)

		if !user.HasCurrentProject() {
			t.Error("User should have a current project after SetCurrentProject")
		}
		if user.CurrentProjectID != projectID {
			t.Errorf("User.CurrentProjectID = %q, want %q", user.CurrentProjectID, projectID)
		}
	})

	t.Run("clear current project", func(t *testing.T) {
		user := &User{
			ID:               "test-id",
			TelegramID:       12345,
			Role:             RoleExecutor,
			CurrentProjectID: "project-123",
		}
		user.ClearCurrentProject()

		if user.HasCurrentProject() {
			t.Error("User should not have a current project after ClearCurrentProject")
		}
		if user.CurrentProjectID != "" {
			t.Errorf("User.CurrentProjectID = %q, want empty", user.CurrentProjectID)
		}
	})

	t.Run("HasCurrentProject returns correct values", func(t *testing.T) {
		user := &User{
			ID:         "test-id",
			TelegramID: 12345,
			Role:       RoleExecutor,
		}

		if user.HasCurrentProject() {
			t.Error("User without CurrentProjectID should return false for HasCurrentProject")
		}

		user.CurrentProjectID = "project-abc"
		if !user.HasCurrentProject() {
			t.Error("User with CurrentProjectID should return true for HasCurrentProject")
		}
	})
}
