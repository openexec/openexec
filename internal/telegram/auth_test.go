package telegram

import (
	"context"
	"errors"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/openexec/openexec/internal/user"
)

func TestNewAuthMiddleware(t *testing.T) {
	store := user.NewMockStore()
	middleware := NewAuthMiddleware(store)

	if middleware == nil {
		t.Fatal("Expected non-nil middleware")
	}

	if middleware.store != store {
		t.Error("Expected store to be set")
	}
}

func TestAuthMiddleware_CheckUserID_AllowedUser(t *testing.T) {
	store := user.NewMockStore()
	ctx := context.Background()

	// Create an allowlisted user
	allowedUser, err := user.NewUser(12345, user.RoleCustomer)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	if err := store.Create(ctx, allowedUser); err != nil {
		t.Fatalf("Failed to store user: %v", err)
	}

	middleware := NewAuthMiddleware(store)
	result := middleware.CheckUserID(ctx, 12345)

	if !result.Allowed {
		t.Error("Expected user to be allowed")
	}
	if result.User == nil {
		t.Error("Expected user to be returned")
	}
	if result.Error != nil {
		t.Errorf("Expected no error, got: %v", result.Error)
	}
	if result.User.TelegramID != 12345 {
		t.Errorf("Expected TelegramID 12345, got %d", result.User.TelegramID)
	}
}

func TestAuthMiddleware_CheckUserID_NotAllowlisted(t *testing.T) {
	store := user.NewMockStore()
	ctx := context.Background()

	middleware := NewAuthMiddleware(store)
	result := middleware.CheckUserID(ctx, 99999)

	if result.Allowed {
		t.Error("Expected user to not be allowed")
	}
	if result.User != nil {
		t.Error("Expected no user to be returned")
	}
	if !errors.Is(result.Error, ErrUserNotAllowlisted) {
		t.Errorf("Expected ErrUserNotAllowlisted, got: %v", result.Error)
	}
}

func TestAuthMiddleware_IsAllowed(t *testing.T) {
	store := user.NewMockStore()
	ctx := context.Background()

	// Create an allowlisted user
	allowedUser, err := user.NewUser(11111, user.RoleProvider)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	if err := store.Create(ctx, allowedUser); err != nil {
		t.Fatalf("Failed to store user: %v", err)
	}

	middleware := NewAuthMiddleware(store)

	// Test allowed user
	if !middleware.IsAllowed(ctx, 11111) {
		t.Error("Expected user 11111 to be allowed")
	}

	// Test non-allowlisted user
	if middleware.IsAllowed(ctx, 22222) {
		t.Error("Expected user 22222 to not be allowed")
	}
}

func TestAuthMiddleware_CheckUpdate_WithMessage(t *testing.T) {
	store := user.NewMockStore()
	ctx := context.Background()

	// Create an allowlisted user
	allowedUser, err := user.NewUser(12345, user.RoleCustomer)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	if err := store.Create(ctx, allowedUser); err != nil {
		t.Fatalf("Failed to store user: %v", err)
	}

	middleware := NewAuthMiddleware(store)

	update := &Update{
		Message: &tgbotapi.Message{
			From: &tgbotapi.User{
				ID: 12345,
			},
		},
	}

	result := middleware.CheckUpdate(ctx, update)

	if !result.Allowed {
		t.Error("Expected user to be allowed")
	}
	if result.User == nil {
		t.Error("Expected user to be returned")
	}
}

func TestAuthMiddleware_CheckUpdate_NotAllowlisted(t *testing.T) {
	store := user.NewMockStore()
	ctx := context.Background()

	middleware := NewAuthMiddleware(store)

	update := &Update{
		Message: &tgbotapi.Message{
			From: &tgbotapi.User{
				ID: 99999,
			},
		},
	}

	result := middleware.CheckUpdate(ctx, update)

	if result.Allowed {
		t.Error("Expected user to not be allowed")
	}
	if !errors.Is(result.Error, ErrUserNotAllowlisted) {
		t.Errorf("Expected ErrUserNotAllowlisted, got: %v", result.Error)
	}
}

func TestAuthMiddleware_CheckUpdate_NoUser(t *testing.T) {
	store := user.NewMockStore()
	ctx := context.Background()

	middleware := NewAuthMiddleware(store)

	// Update with no user
	update := &Update{
		UpdateID: 123,
	}

	result := middleware.CheckUpdate(ctx, update)

	if result.Allowed {
		t.Error("Expected user to not be allowed")
	}
	if !errors.Is(result.Error, ErrNoUserInUpdate) {
		t.Errorf("Expected ErrNoUserInUpdate, got: %v", result.Error)
	}
}

func TestAuthMiddleware_CheckUpdate_NilUpdate(t *testing.T) {
	store := user.NewMockStore()
	ctx := context.Background()

	middleware := NewAuthMiddleware(store)

	result := middleware.CheckUpdate(ctx, nil)

	if result.Allowed {
		t.Error("Expected user to not be allowed")
	}
	if !errors.Is(result.Error, ErrNoUserInUpdate) {
		t.Errorf("Expected ErrNoUserInUpdate, got: %v", result.Error)
	}
}

func TestExtractUser_FromMessage(t *testing.T) {
	update := &Update{
		Message: &tgbotapi.Message{
			From: &tgbotapi.User{ID: 111},
		},
	}

	u := extractUser(update)
	if u == nil || u.ID != 111 {
		t.Errorf("Expected user with ID 111, got: %v", u)
	}
}

func TestExtractUser_FromEditedMessage(t *testing.T) {
	update := &Update{
		EditedMessage: &tgbotapi.Message{
			From: &tgbotapi.User{ID: 222},
		},
	}

	u := extractUser(update)
	if u == nil || u.ID != 222 {
		t.Errorf("Expected user with ID 222, got: %v", u)
	}
}

func TestExtractUser_FromCallbackQuery(t *testing.T) {
	update := &Update{
		CallbackQuery: &tgbotapi.CallbackQuery{
			From: &tgbotapi.User{ID: 333},
		},
	}

	u := extractUser(update)
	if u == nil || u.ID != 333 {
		t.Errorf("Expected user with ID 333, got: %v", u)
	}
}

func TestExtractUser_FromInlineQuery(t *testing.T) {
	update := &Update{
		InlineQuery: &tgbotapi.InlineQuery{
			From: &tgbotapi.User{ID: 444},
		},
	}

	u := extractUser(update)
	if u == nil || u.ID != 444 {
		t.Errorf("Expected user with ID 444, got: %v", u)
	}
}

func TestExtractUser_FromChosenInlineResult(t *testing.T) {
	update := &Update{
		ChosenInlineResult: &tgbotapi.ChosenInlineResult{
			From: &tgbotapi.User{ID: 555},
		},
	}

	u := extractUser(update)
	if u == nil || u.ID != 555 {
		t.Errorf("Expected user with ID 555, got: %v", u)
	}
}

func TestExtractUser_FromShippingQuery(t *testing.T) {
	update := &Update{
		ShippingQuery: &tgbotapi.ShippingQuery{
			From: &tgbotapi.User{ID: 666},
		},
	}

	u := extractUser(update)
	if u == nil || u.ID != 666 {
		t.Errorf("Expected user with ID 666, got: %v", u)
	}
}

func TestExtractUser_FromPreCheckoutQuery(t *testing.T) {
	update := &Update{
		PreCheckoutQuery: &tgbotapi.PreCheckoutQuery{
			From: &tgbotapi.User{ID: 777},
		},
	}

	u := extractUser(update)
	if u == nil || u.ID != 777 {
		t.Errorf("Expected user with ID 777, got: %v", u)
	}
}

func TestExtractUser_FromPollAnswer(t *testing.T) {
	update := &Update{
		PollAnswer: &tgbotapi.PollAnswer{
			User: tgbotapi.User{ID: 888},
		},
	}

	u := extractUser(update)
	if u == nil || u.ID != 888 {
		t.Errorf("Expected user with ID 888, got: %v", u)
	}
}

func TestExtractUser_FromMyChatMember(t *testing.T) {
	update := &Update{
		MyChatMember: &tgbotapi.ChatMemberUpdated{
			From: tgbotapi.User{ID: 999},
		},
	}

	u := extractUser(update)
	if u == nil || u.ID != 999 {
		t.Errorf("Expected user with ID 999, got: %v", u)
	}
}

func TestExtractUser_FromChatMember(t *testing.T) {
	update := &Update{
		ChatMember: &tgbotapi.ChatMemberUpdated{
			From: tgbotapi.User{ID: 1010},
		},
	}

	u := extractUser(update)
	if u == nil || u.ID != 1010 {
		t.Errorf("Expected user with ID 1010, got: %v", u)
	}
}

func TestExtractUser_FromChatJoinRequest(t *testing.T) {
	update := &Update{
		ChatJoinRequest: &tgbotapi.ChatJoinRequest{
			From: tgbotapi.User{ID: 1111},
		},
	}

	u := extractUser(update)
	if u == nil || u.ID != 1111 {
		t.Errorf("Expected user with ID 1111, got: %v", u)
	}
}

func TestExtractUser_NilUpdate(t *testing.T) {
	u := extractUser(nil)
	if u != nil {
		t.Errorf("Expected nil user for nil update, got: %v", u)
	}
}

func TestExtractUser_EmptyUpdate(t *testing.T) {
	update := &Update{}
	u := extractUser(update)
	if u != nil {
		t.Errorf("Expected nil user for empty update, got: %v", u)
	}
}

func TestExtractUser_MessageWithNilFrom(t *testing.T) {
	update := &Update{
		Message: &tgbotapi.Message{
			From: nil,
		},
	}

	u := extractUser(update)
	if u != nil {
		t.Errorf("Expected nil user for message with nil From, got: %v", u)
	}
}

func TestAuthMiddleware_CheckUpdate_DifferentRoles(t *testing.T) {
	testCases := []struct {
		name string
		role user.Role
	}{
		{"Customer", user.RoleCustomer},
		{"Provider", user.RoleProvider},
		{"Admin", user.RoleAdmin},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := user.NewMockStore()
			ctx := context.Background()

			u, err := user.NewUser(12345, tc.role)
			if err != nil {
				t.Fatalf("Failed to create user: %v", err)
			}
			if err := store.Create(ctx, u); err != nil {
				t.Fatalf("Failed to store user: %v", err)
			}

			middleware := NewAuthMiddleware(store)
			update := &Update{
				Message: &tgbotapi.Message{
					From: &tgbotapi.User{ID: 12345},
				},
			}

			result := middleware.CheckUpdate(ctx, update)

			if !result.Allowed {
				t.Error("Expected user to be allowed")
			}
			if result.User.Role != tc.role {
				t.Errorf("Expected role %s, got %s", tc.role, result.User.Role)
			}
		})
	}
}
