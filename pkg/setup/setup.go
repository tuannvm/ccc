package setup

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/charmbracelet/huh"
	configpkg "github.com/tuannvm/ccc/pkg/config"
	"github.com/tuannvm/ccc/pkg/auth"
	"github.com/tuannvm/ccc/pkg/service"
	"github.com/tuannvm/ccc/pkg/telegram"
)

// InstallSkillFunc is a function type for installing the Claude skill/hooks.
// The root package wires this to hooks.InstallSkill().
type InstallSkillFunc func() error

// Setup runs the interactive setup wizard for ccc
func Setup(botToken string, installSkill InstallSkillFunc) error {
	fmt.Println("🚀 Claude Code Companion Setup")
	fmt.Println("==============================")
	fmt.Println()

	// Load existing config if present
	config, _ := configpkg.Load()
	if config == nil {
		config = &configpkg.Config{}
	}
	if config.Sessions == nil {
		config.Sessions = make(map[string]*configpkg.SessionInfo)
	}
	config.BotToken = botToken

	// Stop listener to avoid getUpdates conflict (409 Conflict)
	fmt.Println("Stopping listener...")
	service.StopListenerService()

	// Step 1: Permission mode
	fmt.Println("Step 1/6: Permission mode")
	var permMode string
	err := huh.NewSelect[string]().
		Title("How should remote sessions handle permissions?").
		Description("This controls what happens when Claude Code needs\npermission to run tools in Telegram-controlled sessions.").
		Options(
			huh.NewOption[string](
				"Auto-approve\n"+
					"  All permissions granted automatically. Claude works without\n"+
					"  interruptions. Best for trusted environments where you control\n"+
					"  physical access to your machine.",
				"auto"),
			huh.NewOption[string](
				"OTP (secure)\n"+
					"  Each permission requires a 6-digit TOTP code from your\n"+
					"  authenticator app (Google Authenticator, Authy, etc.).\n"+
					"  Local terminal sessions keep their normal interactive UI.",
				"otp"),
		).
		Value(&permMode).
		Run()
	if err != nil {
		return fmt.Errorf("selection cancelled: %w", err)
	}
	fmt.Println()

	// Step 2: Get chat ID
	fmt.Println("Step 2/6: Connecting to Telegram...")
	fmt.Println("   📱 Send any message to your bot in Telegram")
	fmt.Println("   Waiting...")

	offset := 0
	for {
		resp, err := telegram.TelegramGet(botToken, fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", botToken, offset))
		if err != nil {
			return fmt.Errorf("failed to get updates: %w", err)
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, telegram.MaxResponseSize))
		resp.Body.Close()

		var updates telegram.TelegramUpdate
		if err := json.Unmarshal(body, &updates); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		if !updates.OK {
			return fmt.Errorf("telegram API error - check your bot token")
		}

		for _, update := range updates.Result {
			offset = update.UpdateID + 1
			if update.Message.Chat.ID != 0 {
				config.ChatID = update.Message.Chat.ID
				if err := configpkg.Save(config); err != nil {
					return fmt.Errorf("failed to save config: %w", err)
				}
				fmt.Printf("✅ Connected! (User: @%s)\n\n", update.Message.From.Username)
				goto step2
			}
		}

		time.Sleep(time.Second)
	}

step2:
	// Step 3: Group setup (optional)
	fmt.Println("Step 3/6: Group setup (optional)")
	fmt.Println("   For session topics, create a Telegram group with Topics enabled,")
	fmt.Println("   add your bot as admin, and send a message there.")
	fmt.Println("   Or press Enter to skip...")

	fmt.Println("   Waiting 30 seconds for group message...")

	client := &http.Client{Timeout: 35 * time.Second}
	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=5", config.BotToken, offset)
		resp, err := telegram.TelegramClientGet(client, config.BotToken, reqURL)
		if err != nil {
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, telegram.MaxResponseSize))
		resp.Body.Close()

		var updates telegram.TelegramUpdate
		json.Unmarshal(body, &updates)

		for _, update := range updates.Result {
			offset = update.UpdateID + 1
			chat := update.Message.Chat
			if chat.Type == "supergroup" && update.Message.From.ID == config.ChatID {
				config.GroupID = chat.ID
				configpkg.Save(config)
				fmt.Printf("✅ Group configured!\n\n")
				goto step3
			}
		}
	}
	fmt.Println("⏭️  Skipped (you can run 'ccc setgroup' later)")

step3:
	// Step 4: Install Claude hook and skill
	if err := installSkill(); err != nil {
		fmt.Printf("⚠️  Skill installation failed: %v\n", err)
	} else {
		fmt.Println()
	}

	// Step 5: Install service
	fmt.Println("Step 5/6: Installing background service...")
	if err := service.InstallService(); err != nil {
		fmt.Printf("⚠️  Service installation failed: %v\n", err)
		fmt.Println("   You can start manually with: ccc listen")
	} else {
		fmt.Println()
	}

	// Step 6: Apply permission mode
	fmt.Println("Step 6/6: Configuring permission mode...")
	if permMode == "otp" {
		msg, err := auth.SetupOTP(config)
		if err != nil {
			fmt.Printf("⚠️  OTP setup failed: %v\n", err)
		} else {
			fmt.Println()
			fmt.Println(msg)
			fmt.Println()
			fmt.Println("   Save this secret! You'll need it to approve remote permission requests.")
		}
	} else {
		config.OTPSecret = ""
		if err := configpkg.Save(config); err != nil {
			fmt.Printf("⚠️  Failed to save config: %v\n", err)
		}
		fmt.Println("✅ Auto-approve mode — all remote permissions granted automatically")
	}

	// Done
	fmt.Println()
	fmt.Println("==============================")
	fmt.Println("✅ Setup complete!")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  ccc           Start Claude Code in current directory")
	fmt.Println("  ccc -c        Continue previous session")
	fmt.Println()
	if config.GroupID != 0 {
		fmt.Println("Telegram commands (in your group):")
		fmt.Println("  /new <name>   Create new session")
		fmt.Println("  /list         List sessions")
	} else {
		fmt.Println("To enable Telegram session topics:")
		fmt.Println("  1. Create a group with Topics enabled")
		fmt.Println("  2. Add bot as admin")
		fmt.Println("  3. Run: ccc setgroup")
	}

	fmt.Println()
	fmt.Println("Restarting listener...")
	service.StartListenerService()

	return nil
}

// SetupFromArgs validates CLI args and runs setup with the given installSkill callback.
func SetupFromArgs(args []string, installSkill InstallSkillFunc) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: ccc setup <bot_token>")
	}
	return Setup(args[0], installSkill)
}

// SetGroupAuto loads the config and configures the Telegram group
func SetGroupAuto() error {
	config, err := configpkg.Load()
	if err != nil {
		return err
	}
	return SetGroup(config)
}

// SetGroup configures the Telegram group for session topics
func SetGroup(config *configpkg.Config) error {
	fmt.Println("Send a message in the group where you want to use topics...")
	fmt.Println("(Make sure Topics are enabled in group settings)")

	offset := 0
	client := &http.Client{Timeout: 35 * time.Second}

	for {
		reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", config.BotToken, offset)
		resp, err := telegram.TelegramClientGet(client, config.BotToken, reqURL)
		if err != nil {
			return err
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, telegram.MaxResponseSize))
		resp.Body.Close()

		var updates telegram.TelegramUpdate
		if err := json.Unmarshal(body, &updates); err != nil {
			continue
		}

		for _, update := range updates.Result {
			offset = update.UpdateID + 1
			chat := update.Message.Chat
			if chat.Type == "supergroup" && update.Message.From.ID == config.ChatID {
				config.GroupID = chat.ID
				if err := configpkg.Save(config); err != nil {
					return err
				}
				fmt.Printf("Group set: %d\n", chat.ID)
				fmt.Println("You can now create sessions with: /new <name>")
				return nil
			}
		}
	}
}
