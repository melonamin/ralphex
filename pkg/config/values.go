package config

import (
	"embed"
	"fmt"
	"os"
	"strings"

	"gopkg.in/ini.v1"
)

// config key names used in INI files.
// using constants prevents typos and enables easy searching for key usage.
const (
	keyClaudeCommand        = "claude_command"
	keyClaudeArgs           = "claude_args"
	keyCodexEnabled         = "codex_enabled"
	keyCodexCommand         = "codex_command"
	keyCodexModel           = "codex_model"
	keyCodexReasoningEffort = "codex_reasoning_effort"
	keyCodexTimeoutMs       = "codex_timeout_ms"
	keyCodexSandbox         = "codex_sandbox"
	keyIterationDelayMs     = "iteration_delay_ms"
	keyTaskRetryCount       = "task_retry_count"
	keyPlansDir             = "plans_dir"
	keyWatchDirs            = "watch_dirs"
)

// Values holds scalar configuration values.
// Fields ending in *Set (e.g., CodexEnabledSet) track whether that field was explicitly
// set in config. This allows distinguishing explicit false/0 from "not set", enabling
// proper merge behavior where local config can override global config with zero values.
type Values struct {
	ClaudeCommand        string
	ClaudeArgs           string
	CodexEnabled         bool
	CodexEnabledSet      bool // tracks if codex_enabled was explicitly set
	CodexCommand         string
	CodexModel           string
	CodexReasoningEffort string
	CodexTimeoutMs       int
	CodexTimeoutMsSet    bool // tracks if codex_timeout_ms was explicitly set
	CodexSandbox         string
	IterationDelayMs     int
	IterationDelayMsSet  bool // tracks if iteration_delay_ms was explicitly set
	TaskRetryCount       int
	TaskRetryCountSet    bool // tracks if task_retry_count was explicitly set
	PlansDir             string
	WatchDirs            []string // directories to watch for progress files
}

// valuesLoader implements ValuesLoader with embedded filesystem fallback.
type valuesLoader struct {
	embedFS embed.FS
}

// newValuesLoader creates a new valuesLoader with the given embedded filesystem.
func newValuesLoader(embedFS embed.FS) *valuesLoader {
	return &valuesLoader{embedFS: embedFS}
}

// Load loads values from config files with fallback chain: local → global → embedded.
// localConfigPath and globalConfigPath are full paths to config files (not directories).
//
//nolint:dupl // intentional structural similarity with colorLoader.Load
func (vl *valuesLoader) Load(localConfigPath, globalConfigPath string) (Values, error) {
	// start with embedded defaults
	embedded, err := vl.parseValuesFromEmbedded()
	if err != nil {
		return Values{}, fmt.Errorf("parse embedded defaults: %w", err)
	}

	// parse global config if exists
	global, err := vl.parseValuesFromFile(globalConfigPath)
	if err != nil {
		return Values{}, fmt.Errorf("parse global config: %w", err)
	}

	// parse local config if exists
	local, err := vl.parseValuesFromFile(localConfigPath)
	if err != nil {
		return Values{}, fmt.Errorf("parse local config: %w", err)
	}

	// merge: embedded → global → local (local wins)
	result := embedded
	result.mergeFrom(&global)
	result.mergeFrom(&local)

	return result, nil
}

// parseValuesFromFile reads a config file and parses it into Values.
// returns empty Values (not error) if file doesn't exist.
func (vl *valuesLoader) parseValuesFromFile(path string) (Values, error) {
	if path == "" {
		return Values{}, nil
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is constructed internally
	if err != nil {
		if os.IsNotExist(err) {
			return Values{}, nil
		}
		return Values{}, fmt.Errorf("read config %s: %w", path, err)
	}

	return vl.parseValuesFromBytes(data)
}

// parseValuesFromEmbedded parses values from the embedded defaults/config file.
func (vl *valuesLoader) parseValuesFromEmbedded() (Values, error) {
	data, err := vl.embedFS.ReadFile("defaults/config")
	if err != nil {
		return Values{}, fmt.Errorf("read embedded defaults: %w", err)
	}
	return vl.parseValuesFromBytes(data)
}

// parseValuesFromBytes parses configuration from a byte slice into Values.
func (vl *valuesLoader) parseValuesFromBytes(data []byte) (Values, error) {
	// ignoreInlineComment: true prevents # from being treated as inline comment marker
	cfg, err := ini.LoadSources(ini.LoadOptions{IgnoreInlineComment: true}, data)
	if err != nil {
		return Values{}, fmt.Errorf("parse config: %w", err)
	}

	var values Values
	section := cfg.Section("") // default section (no section header)

	// claude settings
	values.ClaudeCommand = getStringKey(section, keyClaudeCommand)
	values.ClaudeArgs = getStringKey(section, keyClaudeArgs)

	// codex settings
	codexEnabled, codexEnabledSet, err := getBoolKey(section, keyCodexEnabled)
	if err != nil {
		return Values{}, err
	}
	values.CodexEnabled = codexEnabled
	values.CodexEnabledSet = codexEnabledSet

	values.CodexCommand = getStringKey(section, keyCodexCommand)
	values.CodexModel = getStringKey(section, keyCodexModel)
	values.CodexReasoningEffort = getStringKey(section, keyCodexReasoningEffort)

	codexTimeout, codexTimeoutSet, err := getNonNegativeIntKey(section, keyCodexTimeoutMs)
	if err != nil {
		return Values{}, err
	}
	values.CodexTimeoutMs = codexTimeout
	values.CodexTimeoutMsSet = codexTimeoutSet

	values.CodexSandbox = getStringKey(section, keyCodexSandbox)

	// timing settings
	iterDelay, iterDelaySet, err := getNonNegativeIntKey(section, keyIterationDelayMs)
	if err != nil {
		return Values{}, err
	}
	values.IterationDelayMs = iterDelay
	values.IterationDelayMsSet = iterDelaySet

	retryCount, retryCountSet, err := getNonNegativeIntKey(section, keyTaskRetryCount)
	if err != nil {
		return Values{}, err
	}
	values.TaskRetryCount = retryCount
	values.TaskRetryCountSet = retryCountSet

	// paths
	values.PlansDir = getStringKey(section, keyPlansDir)
	values.WatchDirs = getCommaSeparatedKey(section, keyWatchDirs)

	return values, nil
}

// getStringKey returns the string value of a key, or empty string if not found.
// returns empty string if section is nil (defensive check).
func getStringKey(section *ini.Section, keyName string) string {
	if section == nil {
		return ""
	}
	if section.HasKey(keyName) {
		return section.Key(keyName).String()
	}
	return ""
}

// getBoolKey parses a bool key. Returns (value, wasSet, error).
// If key doesn't exist or section is nil, returns (false, false, nil).
func getBoolKey(section *ini.Section, keyName string) (bool, bool, error) {
	if section == nil || !section.HasKey(keyName) {
		return false, false, nil
	}
	val, err := section.Key(keyName).Bool()
	if err != nil {
		return false, false, fmt.Errorf("invalid %s: %w", keyName, err)
	}
	return val, true, nil
}

// getNonNegativeIntKey parses an int key and validates it's non-negative.
// Returns (value, wasSet, error). If key doesn't exist or section is nil, returns (0, false, nil).
func getNonNegativeIntKey(section *ini.Section, keyName string) (int, bool, error) {
	if section == nil || !section.HasKey(keyName) {
		return 0, false, nil
	}
	val, err := section.Key(keyName).Int()
	if err != nil {
		return 0, false, fmt.Errorf("invalid %s: %w", keyName, err)
	}
	if val < 0 {
		return 0, false, fmt.Errorf("invalid %s: must be non-negative, got %d", keyName, val)
	}
	return val, true, nil
}

// getCommaSeparatedKey returns a slice of trimmed strings from a comma-separated key value.
// returns nil if key doesn't exist, is empty, or section is nil.
func getCommaSeparatedKey(section *ini.Section, keyName string) []string {
	val := strings.TrimSpace(getStringKey(section, keyName))
	if val == "" {
		return nil
	}
	return parseCommaSeparatedList(val)
}

// parseCommaSeparatedList splits a comma-separated string into a list of trimmed strings.
func parseCommaSeparatedList(val string) []string {
	parts := strings.Split(val, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// mergeFrom merges non-empty values from src into dst.
func (dst *Values) mergeFrom(src *Values) {
	if src.ClaudeCommand != "" {
		dst.ClaudeCommand = src.ClaudeCommand
	}
	if src.ClaudeArgs != "" {
		dst.ClaudeArgs = src.ClaudeArgs
	}
	if src.CodexEnabledSet {
		dst.CodexEnabled = src.CodexEnabled
		dst.CodexEnabledSet = true
	}
	if src.CodexCommand != "" {
		dst.CodexCommand = src.CodexCommand
	}
	if src.CodexModel != "" {
		dst.CodexModel = src.CodexModel
	}
	if src.CodexReasoningEffort != "" {
		dst.CodexReasoningEffort = src.CodexReasoningEffort
	}
	if src.CodexTimeoutMsSet {
		dst.CodexTimeoutMs = src.CodexTimeoutMs
		dst.CodexTimeoutMsSet = true
	}
	if src.CodexSandbox != "" {
		dst.CodexSandbox = src.CodexSandbox
	}
	if src.IterationDelayMsSet {
		dst.IterationDelayMs = src.IterationDelayMs
		dst.IterationDelayMsSet = true
	}
	if src.TaskRetryCountSet {
		dst.TaskRetryCount = src.TaskRetryCount
		dst.TaskRetryCountSet = true
	}
	if src.PlansDir != "" {
		dst.PlansDir = src.PlansDir
	}
	if len(src.WatchDirs) > 0 {
		dst.WatchDirs = src.WatchDirs
	}
}
