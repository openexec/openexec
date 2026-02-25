package user

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newTestDB creates a new in-memory SQLite database for testing.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	return db
}

func TestNewSQLiteStore(t *testing.T) {
	t.Run("creates store with valid db", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, err := NewSQLiteStore(db)
		if err != nil {
			t.Fatalf("NewSQLiteStore() error = %v", err)
		}
		if store == nil {
			t.Fatal("NewSQLiteStore() returned nil store")
		}
	})

	t.Run("returns error for nil db", func(t *testing.T) {
		_, err := NewSQLiteStore(nil)
		if err == nil {
			t.Fatal("NewSQLiteStore(nil) should return error")
		}
	})
}

func TestSQLiteStore_Create(t *testing.T) {
	t.Run("creates user successfully", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, err := NewSQLiteStore(db)
		if err != nil {
			t.Fatalf("NewSQLiteStore() error = %v", err)
		}

		user, err := NewUser(12345, RoleAdmin)
		if err != nil {
			t.Fatalf("NewUser() error = %v", err)
		}

		err = store.Create(context.Background(), user)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify user was created
		retrieved, err := store.GetByID(context.Background(), user.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.ID != user.ID {
			t.Errorf("retrieved.ID = %v, want %v", retrieved.ID, user.ID)
		}
		if retrieved.TelegramID != user.TelegramID {
			t.Errorf("retrieved.TelegramID = %v, want %v", retrieved.TelegramID, user.TelegramID)
		}
		if retrieved.Role != user.Role {
			t.Errorf("retrieved.Role = %v, want %v", retrieved.Role, user.Role)
		}
	})

	t.Run("returns error for duplicate ID", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, err := NewSQLiteStore(db)
		if err != nil {
			t.Fatalf("NewSQLiteStore() error = %v", err)
		}

		user, _ := NewUser(12345, RoleAdmin)
		_ = store.Create(context.Background(), user)

		// Create another user with same ID but different TelegramID
		user2 := &User{ID: user.ID, TelegramID: 67890, Role: RoleAdmin}
		err = store.Create(context.Background(), user2)
		if err != ErrUserAlreadyExists {
			t.Errorf("Create() error = %v, want ErrUserAlreadyExists", err)
		}
	})

	t.Run("returns error for duplicate TelegramID", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, err := NewSQLiteStore(db)
		if err != nil {
			t.Fatalf("NewSQLiteStore() error = %v", err)
		}

		user1, _ := NewUser(12345, RoleAdmin)
		_ = store.Create(context.Background(), user1)

		user2, _ := NewUser(12345, RoleCustomer)
		err = store.Create(context.Background(), user2)
		if err != ErrUserAlreadyExists {
			t.Errorf("Create() error = %v, want ErrUserAlreadyExists", err)
		}
	})

	t.Run("returns error for invalid user", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, err := NewSQLiteStore(db)
		if err != nil {
			t.Fatalf("NewSQLiteStore() error = %v", err)
		}

		user := &User{ID: "", TelegramID: 12345, Role: RoleAdmin}
		err = store.Create(context.Background(), user)
		if err != ErrInvalidUser {
			t.Errorf("Create() error = %v, want ErrInvalidUser", err)
		}
	})
}

func TestSQLiteStore_GetByID(t *testing.T) {
	t.Run("retrieves existing user", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, _ := NewSQLiteStore(db)
		user, _ := NewUser(12345, RoleExecutor)
		user.CurrentProjectID = "project-123"
		_ = store.Create(context.Background(), user)

		retrieved, err := store.GetByID(context.Background(), user.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.CurrentProjectID != "project-123" {
			t.Errorf("CurrentProjectID = %v, want project-123", retrieved.CurrentProjectID)
		}
	})

	t.Run("returns ErrUserNotFound for non-existent user", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, _ := NewSQLiteStore(db)

		_, err := store.GetByID(context.Background(), "non-existent")
		if err != ErrUserNotFound {
			t.Errorf("GetByID() error = %v, want ErrUserNotFound", err)
		}
	})
}

func TestSQLiteStore_GetByTelegramID(t *testing.T) {
	t.Run("retrieves user by telegram ID", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, _ := NewSQLiteStore(db)
		user, _ := NewUser(12345, RoleProvider)
		_ = store.Create(context.Background(), user)

		retrieved, err := store.GetByTelegramID(context.Background(), 12345)
		if err != nil {
			t.Fatalf("GetByTelegramID() error = %v", err)
		}
		if retrieved.ID != user.ID {
			t.Errorf("retrieved.ID = %v, want %v", retrieved.ID, user.ID)
		}
	})

	t.Run("returns ErrUserNotFound for non-existent telegram ID", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, _ := NewSQLiteStore(db)

		_, err := store.GetByTelegramID(context.Background(), 99999)
		if err != ErrUserNotFound {
			t.Errorf("GetByTelegramID() error = %v, want ErrUserNotFound", err)
		}
	})
}

func TestSQLiteStore_Update(t *testing.T) {
	t.Run("updates existing user", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, _ := NewSQLiteStore(db)
		user, _ := NewUser(12345, RoleCustomer)
		_ = store.Create(context.Background(), user)

		user.Role = RoleAdmin
		user.CurrentProjectID = "new-project"
		err := store.Update(context.Background(), user)
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, _ := store.GetByID(context.Background(), user.ID)
		if retrieved.Role != RoleAdmin {
			t.Errorf("Role = %v, want admin", retrieved.Role)
		}
		if retrieved.CurrentProjectID != "new-project" {
			t.Errorf("CurrentProjectID = %v, want new-project", retrieved.CurrentProjectID)
		}
	})

	t.Run("returns ErrUserNotFound for non-existent user", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, _ := NewSQLiteStore(db)
		user := &User{ID: "non-existent", TelegramID: 12345, Role: RoleAdmin}

		err := store.Update(context.Background(), user)
		if err != ErrUserNotFound {
			t.Errorf("Update() error = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("returns error for duplicate TelegramID", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, _ := NewSQLiteStore(db)

		user1, _ := NewUser(11111, RoleAdmin)
		user2, _ := NewUser(22222, RoleCustomer)
		_ = store.Create(context.Background(), user1)
		_ = store.Create(context.Background(), user2)

		// Try to update user2 with user1's TelegramID
		user2.TelegramID = 11111
		err := store.Update(context.Background(), user2)
		if err != ErrUserAlreadyExists {
			t.Errorf("Update() error = %v, want ErrUserAlreadyExists", err)
		}
	})

	t.Run("returns error for invalid user", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, _ := NewSQLiteStore(db)
		user, _ := NewUser(12345, RoleAdmin)
		_ = store.Create(context.Background(), user)

		user.Role = "invalid"
		err := store.Update(context.Background(), user)
		if err != ErrInvalidUser {
			t.Errorf("Update() error = %v, want ErrInvalidUser", err)
		}
	})
}

func TestSQLiteStore_Delete(t *testing.T) {
	t.Run("deletes existing user", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, _ := NewSQLiteStore(db)
		user, _ := NewUser(12345, RoleAdmin)
		_ = store.Create(context.Background(), user)

		err := store.Delete(context.Background(), user.ID)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err = store.GetByID(context.Background(), user.ID)
		if err != ErrUserNotFound {
			t.Errorf("GetByID() after delete error = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("returns ErrUserNotFound for non-existent user", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, _ := NewSQLiteStore(db)

		err := store.Delete(context.Background(), "non-existent")
		if err != ErrUserNotFound {
			t.Errorf("Delete() error = %v, want ErrUserNotFound", err)
		}
	})
}

func TestSQLiteStore_List(t *testing.T) {
	t.Run("returns empty slice when no users", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, _ := NewSQLiteStore(db)

		users, err := store.List(context.Background())
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if users == nil {
			t.Fatal("List() returned nil, want empty slice")
		}
		if len(users) != 0 {
			t.Errorf("len(users) = %d, want 0", len(users))
		}
	})

	t.Run("returns all users", func(t *testing.T) {
		db := newTestDB(t)
		defer db.Close()

		store, _ := NewSQLiteStore(db)

		user1, _ := NewUser(11111, RoleAdmin)
		user2, _ := NewUser(22222, RoleCustomer)
		user3, _ := NewUser(33333, RoleProvider)
		_ = store.Create(context.Background(), user1)
		_ = store.Create(context.Background(), user2)
		_ = store.Create(context.Background(), user3)

		users, err := store.List(context.Background())
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(users) != 3 {
			t.Errorf("len(users) = %d, want 3", len(users))
		}
	})
}

func TestSQLiteStore_Persistence(t *testing.T) {
	t.Run("data persists across store instances", func(t *testing.T) {
		// Use a file-based database for this test
		db, err := sql.Open("sqlite3", ":memory:?cache=shared")
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		// Create first store and add a user
		store1, _ := NewSQLiteStore(db)
		user, _ := NewUser(12345, RoleAdmin)
		_ = store1.Create(context.Background(), user)

		// Create second store with same db connection
		store2, _ := NewSQLiteStore(db)

		// Verify user exists in second store
		retrieved, err := store2.GetByID(context.Background(), user.ID)
		if err != nil {
			t.Fatalf("GetByID() from second store error = %v", err)
		}
		if retrieved.TelegramID != 12345 {
			t.Errorf("TelegramID = %v, want 12345", retrieved.TelegramID)
		}
	})
}

func TestSQLiteStore_InterfaceCompliance(t *testing.T) {
	// Verify SQLiteStore implements Store interface
	var _ Store = (*SQLiteStore)(nil)
}
