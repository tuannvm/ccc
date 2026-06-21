package config

import (
	"fmt"
	"os"
)

// HandleConfigCommand implements the 'ccc config' subcommand.
// args are os.Args[2:] (everything after "config").
// isOTPEnabled is injected to avoid import cycles (pkg/auth imports pkg/config).
func HandleConfigCommand(args []string, isOTPEnabled func(*Config) bool) {
	cfg, err := Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(args) < 1 {
		// Show current config
		fmt.Printf("projects_dir: %s\n", GetProjectsDir(cfg))
		if cfg.OAuthToken != "" {
			fmt.Println("oauth_token: configured")
		} else {
			fmt.Println("oauth_token: not set")
		}
		if cfg.TranscriptionLang != "" {
			fmt.Printf("transcription_lang: %s\n", cfg.TranscriptionLang)
		} else {
			fmt.Println("transcription_lang: not set (auto-detect)")
		}
		if isOTPEnabled(cfg) {
			fmt.Println("otp: enabled")
		} else {
			fmt.Println("otp: disabled (enable with: ccc setup <bot_token>)")
		}
		fmt.Println("\nUsage: ccc config <key> <value>")
		fmt.Println("  ccc config projects-dir ~/Projects")
		fmt.Println("  ccc config oauth-token <token>")
		fmt.Println("  ccc config transcription-lang es")
		fmt.Println("  ccc config codex-relay true")
		os.Exit(0)
	}

	key := args[0]
	if len(args) < 2 {
		// Show specific key
		switch key {
		case "projects-dir":
			fmt.Println(GetProjectsDir(cfg))
		case "oauth-token":
			if cfg.OAuthToken != "" {
				fmt.Println("configured")
			} else {
				fmt.Println("not set")
			}
		case "bot-token":
			if cfg.BotToken != "" {
				fmt.Println("configured")
			} else {
				fmt.Println("not set")
			}
		case "transcription-lang":
			if cfg.TranscriptionLang != "" {
				fmt.Println(cfg.TranscriptionLang)
			} else {
				fmt.Println("not set (auto-detect)")
			}
		case "codex-relay":
			if cfg.CodexRelay.Enabled {
				transport := cfg.CodexRelay.Transport
				if transport == "" {
					transport = "stdio"
				}
				fmt.Printf("enabled (%s)\n", transport)
			} else {
				fmt.Println("disabled")
			}
		case "otp":
			if isOTPEnabled(cfg) {
				fmt.Println("enabled")
			} else {
				fmt.Println("disabled")
			}
		default:
			fmt.Fprintf(os.Stderr, "Unknown config key: %s\n", key)
			os.Exit(1)
		}
		os.Exit(0)
	}

	value := args[1]
	switch key {
	case "projects-dir":
		cfg.ProjectsDir = value
		if err := Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ projects_dir set to: %s\n", GetProjectsDir(cfg))
	case "oauth-token":
		cfg.OAuthToken = value
		if err := Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ OAuth token saved")
	case "bot-token":
		cfg.BotToken = value
		if err := Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Bot token saved")
	case "transcription-lang":
		cfg.TranscriptionLang = value
		if err := Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Transcription language set to: %s\n", value)
	case "codex-relay":
		switch value {
		case "true", "on", "enabled", "1":
			cfg.CodexRelay.Enabled = true
			cfg.CodexRelay.Transport = "stdio"
		case "false", "off", "disabled", "0":
			cfg.CodexRelay.Enabled = false
		default:
			fmt.Fprintf(os.Stderr, "codex-relay must be true or false\n")
			os.Exit(1)
		}
		if err := Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ codex_relay enabled: %v\n", cfg.CodexRelay.Enabled)
	case "otp":
		fmt.Fprintf(os.Stderr, "Permission mode can only be changed via: ccc setup <bot_token>\n")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "Unknown config key: %s\n", key)
		os.Exit(1)
	}
}
