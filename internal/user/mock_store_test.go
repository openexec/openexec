package user

import (
	"context"
	"errors"
	"testing"
)

func TestMockStore_Create(t *testing.T) {
	ctx := context.Background()

	t.Run("creates new user successfully", func(t *testing.T) {
		store := NewMockStore()
		user, _ := NewUser(12345, RoleCustomer)

		err := store.Create(ctx, user)
		if err != nil {
			t.Fatalf("Create() error = %v, want nil", err)
		}

		// Verify user was stored
		stored, err := store.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if stored.TelegramID != user.TelegramID {
			t.Errorf("stored TelegramID = %v, want %v", stored.TelegramID, user.TelegramID)
		}
	})

	t.Run("rejects duplicate ID", func(t *testing.T) {
		store := NewMockStore()
		user1, _ := NewUser(12345, RoleCustomer)
		user2 := &User{
			ID:         user1.ID, // Same ID
			TelegramID: 67890,
			Role:       RoleProvider,
		}

		_ = store.Create(ctx, user1)
		err := store.Create(ctx, user2)

		if !errors.Is(err, ErrUserAlreadyExists) {
			t.Errorf("Create() error = %v, want ErrUserAlreadyExists", err)
		}
	})

	t.Run("rejects duplicate TelegramID", func(t *testing.T) {
		store := NewMockStore()
		user1, _ := NewUser(12345, RoleCustomer)
		user2, _ := NewUser(12345, RoleProvider) // Same TelegramID

		_ = store.Create(ctx, user1)
		err := store.Create(ctx, user2)

		if !errors.Is(err, ErrUserAlreadyExists) {
			t.Errorf("Create() error = %v, want ErrUserAlreadyExists", err)
		}
	})

	t.Run("rejects invalid user", func(t *testing.T) {
		store := NewMockStore()
		user := &User{
			ID:         "",
			TelegramID: 12345,
			Role:       RoleCustomer,
		}

		err := store.Create(ctx, user)
		if !errors.Is(err, ErrInvalidUser) {
			t.Errorf("Create() error = %v, want ErrInvalidUser", err)
		}
	})
}

func TestMockStore_GetByID(t *testing.T) {
	ctx := context.Background()

	t.Run("returns existing user", func(t *testing.T) {
		store := NewMockStore()
		user, _ := NewUser(12345, RoleCustomer)
		_ = store.Create(ctx, user)

		got, err := store.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v, want nil", err)
		}
		if got.ID != user.ID {
			t.Errorf("GetByID() ID = %v, want %v", got.ID, user.ID)
		}
		if got.TelegramID != user.TelegramID {
			t.Errorf("GetByID() TelegramID = %v, want %v", got.TelegramID, user.TelegramID)
		}
	})

	t.Run("returns error for non-existent user", func(t *testing.T) {
		store := NewMockStore()

		_, err := store.GetByID(ctx, "non-existent")
		if !errors.Is(err, ErrUserNotFound) {
			t.Errorf("GetByID() error = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("returns copy not reference", func(t *testing.T) {
		store := NewMockStore()
		user, _ := NewUser(12345, RoleCustomer)
		_ = store.Create(ctx, user)

		got1, _ := store.GetByID(ctx, user.ID)
		got1.Role = RoleAdmin

		got2, _ := store.GetByID(ctx, user.ID)
		if got2.Role != RoleCustomer {
			t.Error("GetByID() should return a copy, not a reference")
		}
	})
}

func TestMockStore_GetByTelegramID(t *testing.T) {
	ctx := context.Background()

	t.Run("returns existing user", func(t *testing.T) {
		store := NewMockStore()
		user, _ := NewUser(12345, RoleProvider)
		_ = store.Create(ctx, user)

		got, err := store.GetByTelegramID(ctx, 12345)
		if err != nil {
			t.Fatalf("GetByTelegramID() error = %v, want nil", err)
		}
		if got.TelegramID != 12345 {
			t.Errorf("GetByTelegramID() TelegramID = %v, want 12345", got.TelegramID)
		}
	})

	t.Run("returns error for non-existent user", func(t *testing.T) {
		store := NewMockStore()

		_, err := store.GetByTelegramID(ctx, 99999)
		if !errors.Is(err, ErrUserNotFound) {
			t.Errorf("GetByTelegramID() error = %v, want ErrUserNotFound", err)
		}
	})
}

func TestMockStore_Update(t *testing.T) {
	ctx := context.Background()

	t.Run("updates existing user", func(t *testing.T) {
		store := NewMockStore()
		user, _ := NewUser(12345, RoleCustomer)
		_ = store.Create(ctx, user)

		user.Role = RoleProvider
		err := store.Update(ctx, user)
		if err != nil {
			t.Fatalf("Update() error = %v, want nil", err)
		}

		got, _ := store.GetByID(ctx, user.ID)
		if got.Role != RoleProvider {
			t.Errorf("Update() Role = %v, want %v", got.Role, RoleProvider)
		}
	})

	t.Run("updates TelegramID and index", func(t *testing.T) {
		store := NewMockStore()
		user, _ := NewUser(12345, RoleCustomer)
		_ = store.Create(ctx, user)

		user.TelegramID = 67890
		err := store.Update(ctx, user)
		if err != nil {
			t.Fatalf("Update() error = %v, want nil", err)
		}

		// Old TelegramID should not find the user
		_, err = store.GetByTelegramID(ctx, 12345)
		if !errors.Is(err, ErrUserNotFound) {
			t.Error("Update() should remove old TelegramID from index")
		}

		// New TelegramID should find the user
		got, err := store.GetByTelegramID(ctx, 67890)
		if err != nil {
			t.Fatalf("GetByTelegramID() error = %v", err)
		}
		if got.ID != user.ID {
			t.Error("Update() should update TelegramID index")
		}
	})

	t.Run("rejects TelegramID conflict", func(t *testing.T) {
		store := NewMockStore()
		user1, _ := NewUser(12345, RoleCustomer)
		user2, _ := NewUser(67890, RoleProvider)
		_ = store.Create(ctx, user1)
		_ = store.Create(ctx, user2)

		user1.TelegramID = 67890 // Try to take user2's TelegramID
		err := store.Update(ctx, user1)
		if !errors.Is(err, ErrUserAlreadyExists) {
			t.Errorf("Update() error = %v, want ErrUserAlreadyExists", err)
		}
	})

	t.Run("returns error for non-existent user", func(t *testing.T) {
		store := NewMockStore()
		user := &User{
			ID:         "non-existent",
			TelegramID: 12345,
			Role:       RoleCustomer,
		}

		err := store.Update(ctx, user)
		if !errors.Is(err, ErrUserNotFound) {
			t.Errorf("Update() error = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("rejects invalid user", func(t *testing.T) {
		store := NewMockStore()
		user, _ := NewUser(12345, RoleCustomer)
		_ = store.Create(ctx, user)

		user.Role = Role("invalid")
		err := store.Update(ctx, user)
		if !errors.Is(err, ErrInvalidUser) {
			t.Errorf("Update() error = %v, want ErrInvalidUser", err)
		}
	})
}

func TestMockStore_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes existing user", func(t *testing.T) {
		store := NewMockStore()
		user, _ := NewUser(12345, RoleCustomer)
		_ = store.Create(ctx, user)

		err := store.Delete(ctx, user.ID)
		if err != nil {
			t.Fatalf("Delete() error = %v, want nil", err)
		}

		// User should no longer exist
		_, err = store.GetByID(ctx, user.ID)
		if !errors.Is(err, ErrUserNotFound) {
			t.Error("Delete() should remove user")
		}

		// TelegramID index should also be cleaned up
		_, err = store.GetByTelegramID(ctx, 12345)
		if !errors.Is(err, ErrUserNotFound) {
			t.Error("Delete() should remove TelegramID from index")
		}
	})

	t.Run("returns error for non-existent user", func(t *testing.T) {
		store := NewMockStore()

		err := store.Delete(ctx, "non-existent")
		if !errors.Is(err, ErrUserNotFound) {
			t.Errorf("Delete() error = %v, want ErrUserNotFound", err)
		}
	})
}

func TestMockStore_List(t *testing.T) {
	ctx := context.Background()

	t.Run("returns empty slice for empty store", func(t *testing.T) {
		store := NewMockStore()

		users, err := store.List(ctx)
		if err != nil {
			t.Fatalf("List() error = %v, want nil", err)
		}
		if len(users) != 0 {
			t.Errorf("List() len = %v, want 0", len(users))
		}
	})

	t.Run("returns all users", func(t *testing.T) {
		store := NewMockStore()
		user1, _ := NewUser(12345, RoleCustomer)
		user2, _ := NewUser(67890, RoleProvider)
		user3, _ := NewUser(11111, RoleAdmin)
		_ = store.Create(ctx, user1)
		_ = store.Create(ctx, user2)
		_ = store.Create(ctx, user3)

		users, err := store.List(ctx)
		if err != nil {
			t.Fatalf("List() error = %v, want nil", err)
		}
		if len(users) != 3 {
			t.Errorf("List() len = %v, want 3", len(users))
		}
	})

	t.Run("returns copies not references", func(t *testing.T) {
		store := NewMockStore()
		user, _ := NewUser(12345, RoleCustomer)
		_ = store.Create(ctx, user)

		users1, _ := store.List(ctx)
		users1[0].Role = RoleAdmin

		users2, _ := store.List(ctx)
		if users2[0].Role != RoleCustomer {
			t.Error("List() should return copies, not references")
		}
	})
}

func TestMockStore_Concurrency(t *testing.T) {
	ctx := context.Background()
	store := NewMockStore()

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(id int64) {
			user, _ := NewUser(id, RoleCustomer)
			_ = store.Create(ctx, user)
			_, _ = store.GetByID(ctx, user.ID)
			_, _ = store.GetByTelegramID(ctx, id)
			user.Role = RoleProvider
			_ = store.Update(ctx, user)
			_, _ = store.List(ctx)
			_ = store.Delete(ctx, user.ID)
			done <- true
		}(int64(i + 1))
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Store should be empty after all deletes
	users, _ := store.List(ctx)
	if len(users) != 0 {
		t.Errorf("Concurrency test: expected empty store, got %d users", len(users))
	}
}

func TestMockStore_CurrentProjectID(t *testing.T) {
	ctx := context.Background()

	t.Run("creates user with current project ID", func(t *testing.T) {
		store := NewMockStore()
		user := &User{
			ID:               "test-id",
			TelegramID:       12345,
			Role:             RoleExecutor,
			CurrentProjectID: "project-123",
		}

		err := store.Create(ctx, user)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		got, err := store.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if got.CurrentProjectID != "project-123" {
			t.Errorf("Create() CurrentProjectID = %q, want %q", got.CurrentProjectID, "project-123")
		}
	})

	t.Run("updates current project ID", func(t *testing.T) {
		store := NewMockStore()
		user, _ := NewUser(12345, RoleExecutor)
		_ = store.Create(ctx, user)

		user.CurrentProjectID = "new-project-456"
		err := store.Update(ctx, user)
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		got, _ := store.GetByID(ctx, user.ID)
		if got.CurrentProjectID != "new-project-456" {
			t.Errorf("Update() CurrentProjectID = %q, want %q", got.CurrentProjectID, "new-project-456")
		}
	})

	t.Run("GetByTelegramID preserves current project ID", func(t *testing.T) {
		store := NewMockStore()
		user := &User{
			ID:               "test-id",
			TelegramID:       12345,
			Role:             RoleExecutor,
			CurrentProjectID: "project-abc",
		}
		_ = store.Create(ctx, user)

		got, err := store.GetByTelegramID(ctx, 12345)
		if err != nil {
			t.Fatalf("GetByTelegramID() error = %v", err)
		}
		if got.CurrentProjectID != "project-abc" {
			t.Errorf("GetByTelegramID() CurrentProjectID = %q, want %q", got.CurrentProjectID, "project-abc")
		}
	})

	t.Run("List preserves current project ID", func(t *testing.T) {
		store := NewMockStore()
		user := &User{
			ID:               "test-id",
			TelegramID:       12345,
			Role:             RoleExecutor,
			CurrentProjectID: "project-xyz",
		}
		_ = store.Create(ctx, user)

		users, err := store.List(ctx)
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(users) != 1 {
			t.Fatalf("List() len = %v, want 1", len(users))
		}
		if users[0].CurrentProjectID != "project-xyz" {
			t.Errorf("List() CurrentProjectID = %q, want %q", users[0].CurrentProjectID, "project-xyz")
		}
	})

	t.Run("clears current project ID", func(t *testing.T) {
		store := NewMockStore()
		user := &User{
			ID:               "test-id",
			TelegramID:       12345,
			Role:             RoleExecutor,
			CurrentProjectID: "project-123",
		}
		_ = store.Create(ctx, user)

		user.CurrentProjectID = ""
		_ = store.Update(ctx, user)

		got, _ := store.GetByID(ctx, user.ID)
		if got.CurrentProjectID != "" {
			t.Errorf("Update() CurrentProjectID = %q, want empty", got.CurrentProjectID)
		}
	})
}
