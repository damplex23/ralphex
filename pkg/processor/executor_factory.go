package processor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/umputun/ralphex/pkg/config"
	"github.com/umputun/ralphex/pkg/executor"
)

type executorFactory struct{}

func (f *executorFactory) Build(cfg Config, log Logger) (Config, Executors) {
	customExec := cfg.buildCustomExecutor(log)

	if cfg.isCodexExecutor() {
		if cfg.AppConfig.PassGeminiMd {
			maybeEmitGeminiMdSetupHint(log)
		}
		codexTask, codexReview := cfg.buildCodexExecutors(log)
		return cfg, Executors{Task: codexTask, Review: codexReview, Custom: customExec}
	}

	geminiExec, reviewExec := cfg.buildGeminiExecutors(log)
	codexExec := cfg.buildExternalCodexExecutor(log)

	if cfg.CodexEnabled && f.needsCodexBinary(cfg.AppConfig) {
		codexCmd := codexExec.Command
		if codexCmd == "" {
			codexCmd = "codex"
		}
		if _, err := exec.LookPath(codexCmd); err != nil {
			log.Print("warning: codex not found (%s: %v), disabling codex review phase", codexCmd, err)
			cfg.CodexEnabled = false
		}
	}

	return cfg, Executors{Task: geminiExec, Review: reviewExec, External: codexExec, Custom: customExec}
}

// buildGeminiExecutors constructs the gemini executors for task and review phases.
// returns a single executor in the Review slot only when review_model differs from
// the task executor model — otherwise the task executor handles both roles.
func (cfg Config) buildGeminiExecutors(log Logger) (*executor.GeminiExecutor, Executor) {
	geminiExec := &executor.GeminiExecutor{
		OutputHandler: func(text string) {
			log.PrintAligned(text)
		},
		Debug: cfg.Debug,
	}
	cfg.applyGeminiAppConfig(geminiExec)

	taskModel, taskEffort := parseModelEffort(cfg.TaskModel)
	geminiExec.Model, geminiExec.Effort = taskModel, taskEffort

	reviewSpec := cfg.ReviewModel
	if reviewSpec == "" {
		reviewSpec = cfg.TaskModel
	}
	reviewModel, reviewEffort := parseModelEffort(reviewSpec)
	if reviewModel == taskModel && reviewEffort == taskEffort {
		return geminiExec, nil
	}

	reviewExec := &executor.GeminiExecutor{
		OutputHandler: geminiExec.OutputHandler,
		Debug:         cfg.Debug,
		Model:         reviewModel,
		Effort:        reviewEffort,
	}
	cfg.applyGeminiAppConfig(reviewExec)
	return geminiExec, reviewExec
}

// applyGeminiAppConfig copies AppConfig-sourced fields onto a gemini executor.
// no-op when AppConfig is nil.
func (cfg Config) applyGeminiAppConfig(e *executor.GeminiExecutor) {
	if cfg.AppConfig == nil {
		return
	}
	e.Command = cfg.AppConfig.GeminiCommand
	e.Args = cfg.AppConfig.GeminiArgs
	e.ArgsSet = cfg.AppConfig.GeminiArgsSet
	e.ErrorPatterns = cfg.AppConfig.GeminiErrorPatterns
	e.LimitPatterns = cfg.AppConfig.GeminiLimitPatterns
	e.RetryPatterns = cfg.AppConfig.GeminiRetryPatterns
	e.IdleTimeout = cfg.AppConfig.IdleTimeout
	e.PreserveAPIKey = cfg.AppConfig.PreserveGeminiAPIKey
}

// buildExternalCodexExecutor builds the codex executor used for the external review
// phase in gemini mode. MultiAgent stays off (the external review prompt does not use
// spawn_agent) and PassGeminiMd stays off (rejected for gemini mode by applyCodexOverrides).
func (cfg Config) buildExternalCodexExecutor(log Logger) *executor.CodexExecutor {
	e := cfg.newBaseCodexExecutor(log)
	if cfg.AppConfig != nil {
		e.Sandbox = cfg.AppConfig.CodexSandbox
	}
	return e
}

// buildCodexExecutor builds the codex executor used for first-class --codex mode.
// MultiAgent is always enabled so any phase (task, review, finalize) can spawn sub-agents,
// and PassGeminiMd is sourced from config. IdleTimeout is wired here (and only here)
// because the user explicitly opted into --codex; the external-review codex used in
// gemini mode keeps master semantics with no idle timeout.
func (cfg Config) buildCodexExecutor(log Logger) *executor.CodexExecutor {
	e := cfg.newBaseCodexExecutor(log)
	e.MultiAgent = true
	if cfg.AppConfig != nil {
		e.Sandbox = cfg.AppConfig.CodexExecutorSandbox()
		e.PassGeminiMd = cfg.AppConfig.PassGeminiMd
		e.IdleTimeout = cfg.AppConfig.IdleTimeout
	}
	return e
}

// buildCodexExecutors constructs the codex executors for the task and review phases
// in first-class --codex mode. the review slot is non-nil only when the resolved
// review model/effort differs from task — otherwise the task executor handles review
// and finalize too. --task-model / --review-model (and their config equivalents) are
// resolved against codex_model / codex_reasoning_effort: review_model falls back to
// task_model when unset, and an unset spec inherits the codex config defaults.
func (cfg Config) buildCodexExecutors(log Logger) (*executor.CodexExecutor, Executor) {
	var defModel, defEffort string
	if cfg.AppConfig != nil {
		defModel, defEffort = cfg.AppConfig.CodexModel, cfg.AppConfig.CodexReasoningEffort
	}
	taskModel, taskEffort, _ := ResolveCodexModelEffort(cfg.TaskModel, defModel, defEffort)
	reviewModel, reviewEffort := taskModel, taskEffort
	if cfg.ReviewModel != "" {
		reviewModel, reviewEffort, _ = ResolveCodexModelEffort(cfg.ReviewModel, defModel, defEffort)
	}

	taskExec := cfg.buildCodexExecutor(log)
	taskExec.Model, taskExec.ReasoningEffort = taskModel, taskEffort
	if reviewModel == taskModel && reviewEffort == taskEffort {
		return taskExec, nil
	}
	reviewExec := cfg.buildCodexExecutor(log)
	reviewExec.Model, reviewExec.ReasoningEffort = reviewModel, reviewEffort
	return taskExec, reviewExec
}

// newBaseCodexExecutor returns a CodexExecutor populated with the fields shared
// between the external-review and first-class --codex builders. Callers layer on
// Sandbox, MultiAgent, PassGeminiMd, and IdleTimeout as appropriate for their
// role — see buildCodexExecutor (first-class) and buildExternalCodexExecutor
// (gemini mode). IdleTimeout is intentionally NOT set here: applying it to the
// external codex review path silently shortened previously-idle-tolerant
// review sessions for default-gemini users, so it is wired only by
// buildCodexExecutor where the user opted into --codex.
func (cfg Config) newBaseCodexExecutor(log Logger) *executor.CodexExecutor {
	e := &executor.CodexExecutor{
		OutputHandler: func(text string) { log.PrintAligned(text) },
		Debug:         cfg.Debug,
	}
	if cfg.AppConfig == nil {
		return e
	}
	e.Command = cfg.AppConfig.CodexCommand
	e.Model = cfg.AppConfig.CodexModel
	e.ReasoningEffort = cfg.AppConfig.CodexReasoningEffort
	e.TimeoutMs = cfg.AppConfig.CodexTimeoutMs
	e.ErrorPatterns = cfg.AppConfig.CodexErrorPatterns
	e.LimitPatterns = cfg.AppConfig.CodexLimitPatterns
	return e
}

// buildCustomExecutor returns the optional custom external review executor.
// returns nil when no custom_review_script is configured.
func (cfg Config) buildCustomExecutor(log Logger) *executor.CustomExecutor {
	if cfg.AppConfig == nil || cfg.AppConfig.CustomReviewScript == "" {
		return nil
	}
	return &executor.CustomExecutor{
		Script: cfg.AppConfig.CustomReviewScript,
		OutputHandler: func(text string) {
			log.PrintAligned(text)
		},
		ErrorPatterns: cfg.AppConfig.CodexErrorPatterns,
		LimitPatterns: cfg.AppConfig.CodexLimitPatterns,
	}
}

// geminiMdHintOnce ensures the user-level GEMINI.md setup hint emits at most once
// per process, regardless of how many runners or phases are constructed.
var geminiMdHintOnce sync.Once

// maybeEmitGeminiMdSetupHint prints a one-time hint when ~/.gemini/GEMINI.md exists
// but ~/.codex/AGENTS.md does not. ralphex never creates the symlink itself; the
// user owns ~/.codex/. probing errors are swallowed so a missing or unreadable
// home directory simply suppresses the hint.
func maybeEmitGeminiMdSetupHint(log Logger) {
	geminiMdHintOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return
		}
		geminiMd := filepath.Join(home, ".gemini", "GEMINI.md")
		codexAgents := filepath.Join(home, ".codex", "AGENTS.md")
		if !fileExists(geminiMd) {
			return
		}
		if fileExists(codexAgents) {
			return
		}
		log.Print("hint: ~/.gemini/GEMINI.md exists but ~/.codex/AGENTS.md does not. " +
			"to get user-level GEMINI.md content into codex, link it: " +
			"ln -s ~/.gemini/GEMINI.md ~/.codex/AGENTS.md")
	})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// needsCodexBinary returns true when external codex review needs the codex binary.
// first-class codex executor dependency checks happen in cmd/ralphex before runner construction.
func (*executorFactory) needsCodexBinary(appConfig *config.Config) bool {
	if appConfig == nil {
		return true
	}
	switch appConfig.ExternalReviewTool {
	case "custom", "none":
		return false
	default:
		return true
	}
}

// parseModelEffort splits a "model[:effort]" spec into separate parts.
// empty input returns ("", ""). missing colon returns (s, "").
// a leading colon (":high") returns ("", "high"); a trailing colon ("opus:") returns ("opus", "").
// only the first colon is treated as the separator; anything after is passed through as effort.
func parseModelEffort(s string) (model, effort string) {
	model, effort, _ = strings.Cut(s, ":")
	return model, effort
}

// ResolveCodexModelEffort resolves a "model[:effort]" spec against codex default
// model and effort. an empty spec returns the defaults unchanged. each populated
// half of the spec overrides its default. the gemini-only "max" effort is not valid
// for codex: maxDropped reports that the spec requested it (the caller surfaces the
// warning) and the default effort is kept.
func ResolveCodexModelEffort(spec, defModel, defEffort string) (model, effort string, maxDropped bool) {
	model, effort = defModel, defEffort
	if spec == "" {
		return model, effort, false
	}
	m, e := parseModelEffort(spec)
	if m != "" {
		model = m
	}
	if e == "" {
		return model, effort, false
	}
	if strings.EqualFold(e, "max") {
		return model, effort, true
	}
	return model, e, false
}
