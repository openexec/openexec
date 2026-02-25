package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openexec/openexec/internal/health"
	"github.com/openexec/openexec/internal/logging"
	"github.com/openexec/openexec/internal/telegram"
	"github.com/openexec/openexec/internal/twilio"
)

const version = "0.1.0"

func main() {
	// Parse command line flags
	var (
		port              = flag.Int("port", 8090, "HTTP server port")
		telegramToken     = flag.String("telegram-token", os.Getenv("TELEGRAM_BOT_TOKEN"), "Telegram bot token")
		telegramSecret    = flag.String("telegram-secret", os.Getenv("TELEGRAM_WEBHOOK_SECRET"), "Telegram webhook secret")
		twilioSID         = flag.String("twilio-sid", os.Getenv("TWILIO_ACCOUNT_SID"), "Twilio account SID")
		twilioToken       = flag.String("twilio-token", os.Getenv("TWILIO_AUTH_TOKEN"), "Twilio auth token")
		twilioFrom        = flag.String("twilio-from", os.Getenv("TWILIO_FROM_NUMBER"), "Twilio sender number")
		twilioExternalURL = flag.String("twilio-external-url", os.Getenv("TWILIO_EXTERNAL_URL"), "External URL for Twilio signature validation (behind proxy)")
		executionURL      = flag.String("execution-url", os.Getenv("EXECUTION_ENDPOINT"), "OpenExec execution engine URL")
		validateTwilio    = flag.Bool("validate-twilio", true, "Validate Twilio webhook signatures")
		requireExecution  = flag.Bool("require-execution", false, "Require execution API to be available at startup")
	)
	flag.Parse()

	log.Printf("OpenExec Interface Gateway v%s starting...", version)

	// Create health checker
	checker := health.NewChecker()

	// Track enabled channels
	telegramEnabled := false
	twilioEnabled := false

	// Validate Telegram configuration
	if *telegramToken != "" {
		checker.Register(health.Check{
			Name:     "telegram_config",
			Critical: false,
			Run: func(ctx context.Context) (health.Status, string, error) {
				if *telegramToken == "" {
					return health.StatusDegraded, "Telegram token not configured", nil
				}
				// Validate token format (basic check)
				if len(*telegramToken) < 30 {
					return health.StatusFailed, "Telegram token appears invalid (too short)", nil
				}
				return health.StatusOK, "Telegram configuration valid", nil
			},
			Remediation: "Set TELEGRAM_BOT_TOKEN environment variable with a valid bot token",
		})
		telegramEnabled = true
	}

	// Validate Twilio configuration
	if *twilioSID != "" && *twilioToken != "" {
		checker.Register(health.Check{
			Name:     "twilio_config",
			Critical: false,
			Run: func(ctx context.Context) (health.Status, string, error) {
				if *twilioSID == "" || *twilioToken == "" {
					return health.StatusDegraded, "Twilio credentials not configured", nil
				}
				// Validate SID format (starts with AC)
				if len(*twilioSID) < 10 || (*twilioSID)[:2] != "AC" {
					return health.StatusFailed, "Twilio Account SID appears invalid", nil
				}
				// Warn if validation enabled but no external URL configured
				if *validateTwilio && *twilioExternalURL == "" {
					return health.StatusDegraded, "Twilio signature validation enabled but no external URL set (may fail behind proxy)", nil
				}
				return health.StatusOK, "Twilio configuration valid", nil
			},
			Remediation: "Set TWILIO_ACCOUNT_SID, TWILIO_AUTH_TOKEN, and optionally TWILIO_EXTERNAL_URL",
		})
		twilioEnabled = true
	}

	// Check that at least one channel is enabled
	checker.Register(health.Check{
		Name:     "channels",
		Critical: false,
		Run: func(ctx context.Context) (health.Status, string, error) {
			if !telegramEnabled && !twilioEnabled {
				return health.StatusDegraded, "No messaging channels configured", nil
			}
			channels := []string{}
			if telegramEnabled {
				channels = append(channels, "Telegram")
			}
			if twilioEnabled {
				channels = append(channels, "Twilio/WhatsApp")
			}
			return health.StatusOK, fmt.Sprintf("Channels enabled: %v", channels), nil
		},
		Remediation: "Configure at least one messaging channel (Telegram or Twilio)",
	})

	// Check execution API if configured
	if *executionURL != "" {
		checker.Register(health.Check{
			Name:     "execution_api",
			Critical: *requireExecution,
			Run: func(ctx context.Context) (health.Status, string, error) {
				healthURL := *executionURL + "/health"
				ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()

				req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
				if err != nil {
					return health.StatusFailed, fmt.Sprintf("invalid execution URL: %v", err), nil
				}

				client := &http.Client{Timeout: 5 * time.Second}
				resp, err := client.Do(req)
				if err != nil {
					if *requireExecution {
						return health.StatusFailed, fmt.Sprintf("execution API unreachable: %v", err), nil
					}
					return health.StatusDegraded, fmt.Sprintf("execution API unreachable: %v", err), nil
				}
				defer func() { _ = resp.Body.Close() }()

				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					return health.StatusOK, "execution API reachable", nil
				}
				return health.StatusDegraded, fmt.Sprintf("execution API returned status %d", resp.StatusCode), nil
			},
			Remediation: fmt.Sprintf("Ensure execution engine is running at %s", *executionURL),
		})
	}

	// Run preflight checks
	ctx := context.Background()
	if err := checker.RunPreflight(ctx); err != nil {
		log.Fatalf("Preflight checks failed: %v", err)
	}

	log.Println("Preflight checks completed")

	// Create HTTP mux
	mux := http.NewServeMux()

	// Health endpoints
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"status":"ok","version":"%s"}`, version)))
	})
	mux.HandleFunc("/health/details", checker.Handler(true, version))
	mux.HandleFunc("/ready", checker.ReadyHandler())

	// Setup Telegram webhook if configured
	if telegramEnabled {
		bot, err := telegram.New(telegram.Config{
			Token: *telegramToken,
			Debug: os.Getenv("DEBUG") == "true",
		})
		if err != nil {
			log.Printf("Warning: Failed to initialize Telegram bot: %v", err)
			checker.UpdateCheck("telegram_config", health.StatusFailed, fmt.Sprintf("init failed: %v", err))
		} else {
			// Set up update handler
			bot.SetUpdateHandler(func(update telegram.Update) {
				handleTelegramUpdate(bot, update, *executionURL)
			})

			// Create webhook handler
			webhookHandler := telegram.NewWebhookHandler(bot, *telegramSecret)
			mux.Handle("/webhook/telegram", webhookHandler)
			logging.Info("Telegram webhook enabled", "path", "/webhook/telegram")
		}
	} else {
		logging.Info("Telegram webhook disabled (no token configured)")
	}

	// Setup Twilio/WhatsApp webhook if configured
	if twilioEnabled {
		twilioHandler := twilio.NewWebhookHandler(twilio.WebhookConfig{
			AuthToken:         *twilioToken,
			ValidateSignature: *validateTwilio,
			ExternalURL:       *twilioExternalURL,
		})

		// Create Twilio client for sending responses
		var twilioClient *twilio.Client
		if *twilioFrom != "" {
			var err error
			twilioClient, err = twilio.NewClient(twilio.Config{
				AccountSID: *twilioSID,
				AuthToken:  *twilioToken,
				FromNumber: *twilioFrom,
			})
			if err != nil {
				log.Printf("Warning: Failed to create Twilio client: %v", err)
			}
		}

		// Set message handler
		twilioHandler.SetMessageHandler(func(sms *twilio.IncomingSMS) {
			handleWhatsAppMessage(twilioClient, sms, *executionURL)
		})

		mux.Handle("/webhook/whatsapp", twilioHandler)
		logging.Info("WhatsApp webhook enabled", "path", "/webhook/whatsapp")
	} else {
		logging.Info("WhatsApp webhook disabled (no credentials configured)")
	}

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("HTTP server listening on :%d", *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Start periodic health checks if execution URL is configured
	if *executionURL != "" {
		go runPeriodicHealthChecks(ctx, checker, *executionURL)
	}

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logging.Info("Shutting down server...")

	// Mark as not ready
	checker.SetReady(false)

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}

	logging.Info("Server stopped")
}

func runPeriodicHealthChecks(ctx context.Context, checker *health.Checker, executionURL string) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check execution API health
			healthURL := executionURL + "/health"
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)

			req, _ := http.NewRequestWithContext(checkCtx, http.MethodGet, healthURL, nil)
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			cancel()

			if err != nil {
				checker.UpdateCheck("execution_api", health.StatusDegraded, fmt.Sprintf("unreachable: %v", err))
			} else {
				_ = resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					checker.UpdateCheck("execution_api", health.StatusOK, "reachable")
				} else {
					checker.UpdateCheck("execution_api", health.StatusDegraded, fmt.Sprintf("status %d", resp.StatusCode))
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

// handleTelegramUpdate processes incoming Telegram messages
func handleTelegramUpdate(bot *telegram.Bot, update telegram.Update, executionURL string) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	text := update.Message.Text

	logging.Debug("Received Telegram message", "chat_id", chatID, "text", text)

	// Handle commands
	if update.Message.IsCommand() {
		switch update.Message.Command() {
		case "start":
			_, _ = bot.SendMessage(chatID, "Welcome to OpenExec! I'll notify you about task progress and ask for approvals when needed.")
		case "status":
			_, _ = bot.SendMessage(chatID, "Checking execution status...")
			// Query execution engine for status if URL configured
			if executionURL != "" {
				status := queryExecutionStatus(executionURL)
				_, _ = bot.SendMessage(chatID, status)
			}
		case "help":
			help := `Available commands:
/start - Start the bot
/status - Check current execution status
/approve - Approve pending action
/reject - Reject pending action
/help - Show this help message`
			_, _ = bot.SendMessage(chatID, help)
		default:
			_, _ = bot.SendMessage(chatID, "Unknown command. Use /help for available commands.")
		}
		return
	}

	// Handle regular messages (could be responses to prompts)
	_, _ = bot.SendMessage(chatID, "Message received. Processing...")
}

// handleWhatsAppMessage processes incoming WhatsApp messages
func handleWhatsAppMessage(client *twilio.Client, sms *twilio.IncomingSMS, executionURL string) {
	logging.Debug("Received WhatsApp message",
		"from", sms.From,
		"to", sms.To,
		"body", sms.Body,
	)

	if client == nil {
		logging.Warn("Cannot respond to WhatsApp: no client configured")
		return
	}

	// Process the message
	response := processMessage(sms.Body, executionURL)

	// Send response
	_, err := client.SendWhatsAppMessage(twilio.SendMessageRequest{
		To:   sms.From,
		Body: response,
	})
	if err != nil {
		logging.Error("Failed to send WhatsApp response", "error", err)
	}
}

// processMessage handles incoming message text and returns a response
func processMessage(text string, executionURL string) string {
	// Simple command handling
	switch text {
	case "status", "STATUS":
		if executionURL != "" {
			return queryExecutionStatus(executionURL)
		}
		return "Status check unavailable (no execution API configured)"
	case "help", "HELP":
		return `OpenExec Commands:
- status: Check current task status
- approve: Approve pending action
- reject: Reject pending action
- help: Show this message`
	case "approve", "APPROVE", "yes", "YES", "y", "Y":
		return "Approval noted. Processing..."
	case "reject", "REJECT", "no", "NO", "n", "N":
		return "Rejection noted. Processing..."
	default:
		return "Message received. Reply with 'help' for available commands."
	}
}

// queryExecutionStatus queries the execution API for current status
func queryExecutionStatus(executionURL string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, executionURL+"/api/v1/loops", nil)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("Cannot reach execution API: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("Execution API returned status %d", resp.StatusCode)
	}

	// Simple response for now
	return "Execution API is healthy. Use web dashboard for detailed status."
}
