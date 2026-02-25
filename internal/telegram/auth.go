package telegram

import (
	"context"
	"errors"

	"github.com/openexec/openexec/internal/user"
)

// Common errors returned by auth middleware.
var (
	ErrUserNotAllowlisted = errors.New("user not in allowlist")
	ErrNoUserInUpdate     = errors.New("no user found in update")
)

// AuthMiddleware checks incoming Telegram updates against allowlisted users.
type AuthMiddleware struct {
	store user.Store
}

// NewAuthMiddleware creates a new auth middleware with the given user store.
func NewAuthMiddleware(store user.Store) *AuthMiddleware {
	return &AuthMiddleware{
		store: store,
	}
}

// AuthResult contains the result of an authorization check.
type AuthResult struct {
	Allowed bool
	User    *user.User
	Error   error
}

// CheckUpdate verifies that the user in the Telegram update is allowlisted.
// It extracts the user from the update and checks if they exist in the store.
func (m *AuthMiddleware) CheckUpdate(ctx context.Context, update *Update) AuthResult {
	telegramUser := extractUser(update)
	if telegramUser == nil {
		return AuthResult{
			Allowed: false,
			Error:   ErrNoUserInUpdate,
		}
	}

	return m.CheckUserID(ctx, telegramUser.ID)
}

// CheckUserID verifies that the given Telegram user ID is allowlisted.
func (m *AuthMiddleware) CheckUserID(ctx context.Context, telegramID int64) AuthResult {
	storedUser, err := m.store.GetByTelegramID(ctx, telegramID)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return AuthResult{
				Allowed: false,
				Error:   ErrUserNotAllowlisted,
			}
		}
		// Other store errors
		return AuthResult{
			Allowed: false,
			Error:   err,
		}
	}

	return AuthResult{
		Allowed: true,
		User:    storedUser,
	}
}

// IsAllowed is a convenience method that returns true if the user is allowlisted.
func (m *AuthMiddleware) IsAllowed(ctx context.Context, telegramID int64) bool {
	result := m.CheckUserID(ctx, telegramID)
	return result.Allowed
}

// extractUser extracts the Telegram user from an update.
// It checks various update types (message, edited_message, callback_query, etc.)
func extractUser(update *Update) *User {
	if update == nil {
		return nil
	}

	// Check message
	if update.Message != nil && update.Message.From != nil {
		return update.Message.From
	}

	// Check edited message
	if update.EditedMessage != nil && update.EditedMessage.From != nil {
		return update.EditedMessage.From
	}

	// Check channel post (channels don't have From, but forwarded might)
	if update.ChannelPost != nil && update.ChannelPost.From != nil {
		return update.ChannelPost.From
	}

	// Check edited channel post
	if update.EditedChannelPost != nil && update.EditedChannelPost.From != nil {
		return update.EditedChannelPost.From
	}

	// Check callback query
	if update.CallbackQuery != nil && update.CallbackQuery.From != nil {
		return update.CallbackQuery.From
	}

	// Check inline query
	if update.InlineQuery != nil && update.InlineQuery.From != nil {
		return update.InlineQuery.From
	}

	// Check chosen inline result
	if update.ChosenInlineResult != nil && update.ChosenInlineResult.From != nil {
		return update.ChosenInlineResult.From
	}

	// Check shipping query
	if update.ShippingQuery != nil && update.ShippingQuery.From != nil {
		return update.ShippingQuery.From
	}

	// Check pre-checkout query
	if update.PreCheckoutQuery != nil && update.PreCheckoutQuery.From != nil {
		return update.PreCheckoutQuery.From
	}

	// Check poll answer (User is a value type, not pointer)
	if update.PollAnswer != nil && update.PollAnswer.User.ID != 0 {
		return &update.PollAnswer.User
	}

	// Check my chat member update (From is a value type, not pointer)
	if update.MyChatMember != nil && update.MyChatMember.From.ID != 0 {
		return &update.MyChatMember.From
	}

	// Check chat member update (From is a value type, not pointer)
	if update.ChatMember != nil && update.ChatMember.From.ID != 0 {
		return &update.ChatMember.From
	}

	// Check chat join request (From is a value type, not pointer)
	if update.ChatJoinRequest != nil && update.ChatJoinRequest.From.ID != 0 {
		return &update.ChatJoinRequest.From
	}

	return nil
}
