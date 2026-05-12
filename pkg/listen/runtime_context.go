package listen

import (
	"os"
	"os/exec"
	"strconv"
	"strings"

	configpkg "github.com/tuannvm/ccc/pkg/config"
	providerpkg "github.com/tuannvm/ccc/pkg/provider"
)

type agentRuntimeContext struct {
	ProviderName string
	SessionID    string
}

func detectCurrentAgentContext(cfg *configpkg.Config) agentRuntimeContext {
	ctx := agentRuntimeContext{
		ProviderName: strings.TrimSpace(os.Getenv("CCC_AGENT_PROVIDER")),
		SessionID:    firstNonEmptyEnv("CCC_AGENT_SESSION_ID", "CLAUDE_SESSION_ID", "CODEX_SESSION_ID", "CODEX_THREAD_ID"),
	}
	if ctx.ProviderName == "" {
		ctx.ProviderName = providerFromRuntimeEnv(cfg)
	}
	if ctx.ProviderName == "" {
		ctx.ProviderName = providerFromParentProcess(cfg)
	}
	return ctx
}

func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func providerFromRuntimeEnv(cfg *configpkg.Config) string {
	if os.Getenv("CODEX_SANDBOX_NETWORK_DISABLED") != "" || os.Getenv("CODEX_SESSION_ID") != "" || os.Getenv("CODEX_THREAD_ID") != "" {
		return bestProviderForBackend(cfg, providerpkg.BackendCodex)
	}
	if os.Getenv("CLAUDE_CODE_CONFIG_DIR") != "" || os.Getenv("CLAUDE_CONFIG_DIR") != "" {
		return bestProviderForBackend(cfg, providerpkg.BackendClaude)
	}
	return ""
}

func providerFromParentProcess(cfg *configpkg.Config) string {
	pid := os.Getppid()
	seen := map[int]bool{}
	for pid > 1 && !seen[pid] {
		seen[pid] = true
		cmdline, ppid := processCommandLine(pid)
		lower := strings.ToLower(cmdline)
		if strings.Contains(lower, "codex") && !strings.Contains(lower, "ccc ") {
			return bestProviderForBackend(cfg, providerpkg.BackendCodex)
		}
		if strings.Contains(lower, "claude") && !strings.Contains(lower, "ccc ") {
			return bestProviderForBackend(cfg, providerpkg.BackendClaude)
		}
		if ppid <= 1 || ppid == pid {
			break
		}
		pid = ppid
	}
	return ""
}

func processCommandLine(pid int) (string, int) {
	cmd := exec.Command("ps", "-o", "ppid=", "-o", "command=", "-p", strconv.Itoa(pid))
	out, err := cmd.Output()
	if err != nil {
		return "", 0
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return "", 0
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return line, 0
	}
	ppid, _ := strconv.Atoi(fields[0])
	cmdline := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
	return cmdline, ppid
}

func bestProviderForBackend(cfg *configpkg.Config, backend string) string {
	if cfg != nil {
		if cfg.ActiveProvider != "" && providerpkg.IsCodexBackend(providerpkg.BackendForName(cfg, cfg.ActiveProvider)) == providerpkg.IsCodexBackend(backend) {
			return cfg.ActiveProvider
		}
		for _, name := range providerpkg.GetProviderNames(cfg) {
			if providerpkg.BackendForName(cfg, name) == backend {
				return name
			}
		}
	}
	if providerpkg.IsCodexBackend(backend) {
		return providerpkg.BackendCodex
	}
	return builtinProviderName
}
