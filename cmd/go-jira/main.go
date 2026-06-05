package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var (
	Version string
	Commit  string

	// DefaultOAuthClientID is injected at build time via -ldflags (see
	// .goreleaser.yaml / Makefile). It is the company-wide OAuth client baked
	// into the binary; the flow is a public PKCE client, so there is no secret.
	// Runtime resolution order is env var > CLI flag > this embedded default.
	DefaultOAuthClientID string
)

// Flag names shared between registration and lookup. Keeping them here avoids
// stringly-typed typo risk across files.
const (
	flagEnvFile      = "env-file"
	flagBaseURL      = "base-url"
	flagInsecure     = "insecure"
	flagUsername     = "username"
	flagPassword     = "password"
	flagToken        = "token"
	flagRef          = "ref"
	flagIssueFormat  = "issue-format"
	flagToTransition = "to-transition"
	flagResolution   = "resolution"
	flagComment      = "comment"
	flagAssignee     = "assignee"
	flagMarkdown     = "markdown"
	flagDebug        = "debug"

	// Global presentation flags, registered as persistent flags on the root.
	flagQuiet   = "quiet"
	flagNoColor = "no-color"

	// Data subcommand flags (search/create/update/get/sprints/boards/link).
	flagOutput      = "output"
	flagEpicField   = "epic-field"
	flagSprintField = "sprint-field"
	flagJQL         = "jql"
	flagFields      = "fields"
	flagLimit       = "limit"
	flagProject     = "project"
	flagSummary     = "summary"
	flagDescription = "description"
	flagComponents  = "components"
	flagLabels      = "labels"
	flagEpic        = "epic"
	flagSprint      = "sprint"
	flagKey         = "key"
	flagBoardID     = "board-id"
	flagState       = "state"
	flagBoardType   = "type"
	flagFrom        = "from"
	flagTo          = "to"
	flagLinkType    = "link-type"

	// OAuth-related flags.
	flagClientID      = "client-id"
	flagCallbackPort  = "callback-port"
	flagCallbackCert  = "callback-cert"
	flagCallbackKey   = "callback-key"
	flagCallbackHTTPS = "callback-https"
	flagScope         = "scope"
	flagTimeout       = "timeout"
	flagConfirm       = "confirm"

	// Token refresh broker flags. --broker-url / --broker-token are client-side
	// (route refresh through the broker); --listen / --tls-cert / --tls-key are
	// broker-server-side (the `broker serve` subcommand).
	flagBrokerURL   = "broker-url"
	flagBrokerToken = "broker-token"
	flagListen      = "listen"
	flagTLSCert     = "tls-cert"
	flagTLSKey      = "tls-key"
)

// OAuth environment variables. Unlike the action config (which uses the
// INPUT_<KEY>/<KEY> GitHub Actions convention via util.GetGlobalValue), these
// use fixed JIRA_-prefixed names matching the documented CI/CD contract.
const (
	envOAuthClientID           = "JIRA_OAUTH_CLIENT_ID"
	envOAuthRefreshToken       = "JIRA_OAUTH_REFRESH_TOKEN"        //nolint:gosec // env var name, not a secret
	envOAuthRefreshTokenOutput = "JIRA_OAUTH_REFRESH_TOKEN_OUTPUT" //nolint:gosec // env var name, not a secret
	envOAuthCallbackPort       = "JIRA_OAUTH_CALLBACK_PORT"
	envOAuthCallbackCert       = "JIRA_OAUTH_CALLBACK_CERT"
	envOAuthCallbackKey        = "JIRA_OAUTH_CALLBACK_KEY"
	envOAuthCallbackHTTPS      = "JIRA_OAUTH_CALLBACK_HTTPS"
	envMasterPassword          = "JIRA_MASTER_PASSWORD"

	// Token refresh broker env vars.
	//   - envBrokerURL is client-side: route refresh through this broker.
	//   - envBrokerToken is the shared caller bearer token: the client sends it,
	//     and the broker enforces it only when set there too.
	//   - envOAuthClientSecret is broker-ONLY: the confidential client_secret,
	//     injected from a K8s Secret / Vault. It must never be set on a client.
	//   - envBrokerListen / envBrokerTLSCert / envBrokerTLSKey configure the
	//     broker's own HTTP(S) listener.
	envBrokerURL         = "JIRA_TOKEN_BROKER_URL"
	envBrokerToken       = "JIRA_BROKER_TOKEN"        //nolint:gosec // env var name, not a secret
	envOAuthClientSecret = "JIRA_OAUTH_CLIENT_SECRET" //nolint:gosec // env var name, not a secret
	envBrokerListen      = "JIRA_BROKER_LISTEN"
	envBrokerTLSCert     = "JIRA_BROKER_TLS_CERT"
	envBrokerTLSKey      = "JIRA_BROKER_TLS_KEY"

	// JIRA_-prefixed aliases for the core auth/config fields, matching the env
	// naming used throughout the docs and the auth-resolver error message. The
	// action config still resolves these via the INPUT_<KEY>/<KEY> convention;
	// these are additional fallbacks (lowest precedence) so the documented
	// JIRA_* examples work as written.
	envBaseURL  = "JIRA_BASE_URL"
	envUsername = "JIRA_USERNAME"
	envPassword = "JIRA_PASSWORD"
	envToken    = "JIRA_TOKEN"
	envInsecure = "JIRA_INSECURE"
)

const (
	defaultCallbackPort = 8765
	defaultScope        = "WRITE"

	// defaultBrokerListen is the broker server's default listen address. In
	// Kubernetes the Service maps a stable port to this container port.
	defaultBrokerListen = ":8080"

	// Default custom field IDs used by the data subcommands that reference
	// epic/sprint (create, update, and search). These match the documented Jira
	// Server/DC layout (Epic Link / Sprint) and can be overridden per instance
	// via --epic-field / --sprint-field or the EPIC_FIELD / SPRINT_FIELD env vars.
	defaultEpicField   = "customfield_10101"
	defaultSprintField = "customfield_10100"
)

// statusKey is the structured-log / JSON field name reused across status
// messages and command results. Kept as a constant so the repeated literal
// satisfies goconst.
const statusKey = "status"

// nameKey is the "name" field used to build the Jira REST reference objects
// (e.g. {"name": ...} for assignee and components). Kept as a constant so the
// repeated literal satisfies goconst.
const nameKey = "name"

func main() {
	diag := &requestDiag{}
	ctx := withDiag(context.Background(), diag)
	root := newRootCmd()
	// Reject control characters in the raw argv before cobra routes the command.
	// This runs ahead of Execute so it also covers commands cobra handles
	// specially (e.g. completion), which bypass the root PersistentPreRunE.
	if err := validateNoControlChars(os.Args[1:]); err != nil {
		ce := classify(err, diag)
		addHint(ce, root)
		emitError(ce)
		os.Exit(ce.code)
	}
	if err := root.ExecuteContext(ctx); err != nil {
		ce := classify(err, diag)
		addHint(ce, root)
		emitError(ce)
		os.Exit(ce.code)
	}
}

// versionString returns the clean semver-ish build version, or "dev" for an
// unstamped local build. Kept free of decorative text so `--version` emits a
// single machine-parseable token; build metadata (commit) is exposed via the
// `schema` command instead.
func versionString() string {
	if Version == "" {
		return "dev"
	}
	return Version
}

// Command group IDs used to categorize subcommands in the root help output.
// Grouping keeps the (necessarily large) command set scannable for both humans
// and agents without changing any invocation path.
const (
	groupRun    = "run"
	groupIssues = "issues"
	groupAgile  = "agile"
	groupAuth   = "auth"
	groupConfig = "config"
)

// newRootCmd builds the root command and registers every subcommand. A fresh
// command is built on each call so tests get clean flag state.
//
// Running go-jira with no subcommand prints the help page. As of v1.0 (breaking
// change) the previous bare-command action behavior now lives under
// `go-jira run`.
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "go-jira",
		Short: "Jira integration CLI with OAuth, Basic, and Bearer auth",
		Long: `Jira integration CLI with OAuth, Basic, and Bearer auth.

Exit codes (for scripts and agents):
  0  success
  1  generic runtime error
  2  usage error (bad flags or arguments)
  3  authentication/authorization failure (HTTP 401/403)
  4  rate limited (HTTP 429)

On failure a structured JSON error object is written to stderr, e.g.
  {"error":{"kind":"rate_limit","message":"...","exit_code":4,"status_code":429,"retry_after":"30"}}
Rate-limit responses surface the server's Retry-After hint in retry_after;
requests are not retried automatically.

Composability:
  Diagnostics go to stderr, results to stdout. Use --quiet to drop the
  informational stderr logs, and --no-color (or the NO_COLOR env var) to
  disable ANSI color. Text-bearing flags (--ref, --comment, --description,
  --jql) accept "-" to read the value from stdin, e.g.
    git log -1 --format=%B | go-jira run --ref - --to-transition Done`,
		Example: `  # Show the authenticated user and active auth mode
  go-jira whoami

  # Search issues with JQL and emit JSON for an agent to parse
  go-jira search --jql 'project = GAIA AND status = "In Progress"' --output json

  # Create a Task and capture its key
  go-jira create --project GAIA --summary "Investigate flaky test"

  # Transition every issue referenced in the latest commit message
  git log -1 --format=%B | go-jira run --ref - --to-transition Done

  # Discover the runtime command/flag schema (for agents)
  go-jira schema --output json`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       versionString(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &cliError{code: exitUsage, kind: kindUsage, message: err.Error(), err: err}
	})
	cmd.SetVersionTemplate("{{.Version}}\n")

	// Presentation flags shared by every subcommand. PersistentPreRunE installs
	// the matching slog handler before any command logs.
	cmd.PersistentFlags().BoolP(flagQuiet, "q", false,
		"Suppress informational logs on stderr; warnings, errors, and result output remain")
	cmd.PersistentFlags().Bool(flagNoColor, false,
		"Disable ANSI color in log output (also honored via the NO_COLOR env var)")
	cmd.PersistentFlags().Duration(flagTimeout, 0,
		"Maximum time to wait for the operation to complete, e.g. 30s or 2m; "+
			"0 uses the per-command default so agents can enforce a time budget")
	cmd.PersistentPreRunE = func(c *cobra.Command, _ []string) error {
		quiet, _ := c.Flags().GetBool(flagQuiet)
		noColor, _ := c.Flags().GetBool(flagNoColor)
		setupLogging(quiet, noColor)
		return nil
	}

	cmd.AddGroup(
		&cobra.Group{ID: groupRun, Title: "Automation:"},
		&cobra.Group{ID: groupIssues, Title: "Issues:"},
		&cobra.Group{ID: groupAgile, Title: "Agile (boards/sprints/epics):"},
		&cobra.Group{ID: groupAuth, Title: "Authentication:"},
		&cobra.Group{ID: groupConfig, Title: "Configuration & introspection:"},
	)

	cmd.AddCommand(
		newRunCmd(),
		newLoginCmd(),
		newLogoutCmd(),
		newWhoamiCmd(),
		newTokenCmd(),
		newBrokerCmd(),
		newConfigCmd(),
		newSearchCmd(),
		newCreateCmd(),
		newUpdateCmd(),
		newGetCmd(),
		newSprintsCmd(),
		newEpicsCmd(),
		newBoardsCmd(),
		newLinkCmd(),
		newSchemaCmd(),
		newVersionCmd(),
	)
	return cmd
}

// validateNoControlChars rejects any argument carrying a control character
// (ASCII < 0x20) other than tab, newline, or carriage return. Those bytes —
// NUL, ESC, backspace, and friends — have no legitimate place in a CLI argument
// and are a classic terminal-escape / log-injection vector, so the whole
// invocation is refused as a usage error before any command runs. Tab/CR/LF are
// allowed because text-bearing flags (--comment, --description) may carry them.
func validateNoControlChars(args []string) error {
	for _, a := range args {
		for _, r := range a {
			if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
				return &cliError{
					code: exitUsage,
					kind: kindUsage,
					message: fmt.Sprintf(
						"argument contains a disallowed control character (0x%02X)", r),
				}
			}
		}
	}
	return nil
}

// addOutputFlag registers the shared --output flag for the data subcommands.
func addOutputFlag(cmd *cobra.Command) {
	cmd.Flags().String(flagOutput, outputJSON,
		"Output format: json|text (env: OUTPUT / INPUT_OUTPUT)")
}

// addCustomFieldFlags registers the configurable epic-link and sprint custom
// field IDs. These are consumed by create and update (when setting the fields)
// and by search (which appends them to the default field selection). Field IDs
// vary per Jira instance, so they are overridable.
func addCustomFieldFlags(cmd *cobra.Command) {
	cmd.Flags().String(flagEpicField, defaultEpicField,
		"Epic Link custom field ID (env: EPIC_FIELD / INPUT_EPIC_FIELD)")
	cmd.Flags().String(flagSprintField, defaultSprintField,
		"Sprint custom field ID (env: SPRINT_FIELD / INPUT_SPRINT_FIELD)")
}

// addAuthFlags registers the bearer/basic credential flags shared by the data
// subcommands, mirroring run/whoami. OAuth and common flags are added
// separately by each command.
func addAuthFlags(cmd *cobra.Command) {
	cmd.Flags().String(flagUsername, "", "Jira username (env: USERNAME / INPUT_USERNAME)")
	cmd.Flags().String(flagPassword, "", "Jira password (prefer env: PASSWORD / INPUT_PASSWORD)")
	cmd.Flags().String(flagToken, "", "Jira API token (prefer env: TOKEN / INPUT_TOKEN)")
}

// addEditableIssueFlags registers the issue field flags shared by create and
// update: the fields a caller can set on an issue. create marks --summary
// required and adds --project on top; update treats every flag as an optional
// partial edit.
func addEditableIssueFlags(cmd *cobra.Command) {
	cmd.Flags().String(flagSummary, "", "Issue summary line")
	cmd.Flags().String(flagAssignee, "", "Assignee login name")
	cmd.Flags().String(flagDescription, "", `Issue description body (pass "-" to read from stdin)`)
	cmd.Flags().String(flagComponents, "", "Comma-separated component names")
	cmd.Flags().String(flagLabels, "", "Comma-separated labels")
	cmd.Flags().String(flagEpic, "", "Epic key for the epic-link field, e.g. GAIA-42")
	cmd.Flags().Int(flagSprint, 0, "Sprint ID for the sprint field")
}

// addCommonFlags registers flags shared by all subcommands that talk to Jira.
func addCommonFlags(cmd *cobra.Command) {
	cmd.Flags().String(flagEnvFile, ".env", "Read in a file of environment variables")
	cmd.Flags().String(flagBaseURL, "", "Jira base URL (env: BASE_URL / INPUT_BASE_URL)")
	cmd.Flags().
		Bool(flagInsecure, false, "Skip TLS verification (env: INSECURE / INPUT_INSECURE)")
	cmd.Flags().Bool(flagDebug, false, "Dump resolved configuration (env: DEBUG / INPUT_DEBUG)")
}

// addOAuthFlags registers the OAuth client flags shared by login and run.
func addOAuthFlags(cmd *cobra.Command) {
	cmd.Flags().String(flagClientID, "", "OAuth client ID (env: "+envOAuthClientID+")")
	cmd.Flags().
		Int(flagCallbackPort, defaultCallbackPort, "Local OAuth callback port (env: "+envOAuthCallbackPort+")")
	cmd.Flags().String(flagCallbackCert, "",
		"TLS cert file for an https callback server (env: "+envOAuthCallbackCert+
			"); requires --"+flagCallbackKey)
	cmd.Flags().String(flagCallbackKey, "",
		"TLS key file for an https callback server (env: "+envOAuthCallbackKey+
			"); requires --"+flagCallbackCert)
	cmd.Flags().Bool(flagCallbackHTTPS, false,
		"Serve the https callback with an auto-generated in-memory loopback cert, so "+
			"no cert/key files are needed (env: "+envOAuthCallbackHTTPS+
			"); the browser shows a one-time security warning to accept")
	cmd.Flags().String(flagScope, defaultScope, "OAuth scope to request")
	cmd.Flags().String(flagBrokerURL, "",
		"Token refresh broker base URL; when set, refresh is routed through the "+
			"broker instead of calling Jira directly (env: "+envBrokerURL+")")
	cmd.Flags().String(flagBrokerToken, "",
		"Optional bearer token sent to the broker (env: "+envBrokerToken+")")
}

// loadEnvFile resolves and loads an env file, logging the absolute path that
// was loaded. An explicitly-passed --env-file that is missing is a hard error;
// the default .env is silently skipped when absent.
func loadEnvFile(envfile string, explicit bool) error {
	if envfile == "" {
		return nil
	}
	abs, err := filepath.Abs(envfile)
	if err != nil {
		return fmt.Errorf("resolve env file path: %w", err)
	}
	info, statErr := os.Stat(abs)
	if statErr != nil || info.IsDir() {
		if explicit {
			return fmt.Errorf("env file not found: %s", abs)
		}
		return nil
	}
	if err := godotenv.Load(abs); err != nil {
		return fmt.Errorf("load env file %s: %w", abs, err)
	}
	slog.Info("loaded env file", "path", abs)
	return nil
}

// loadEnvFromCmd loads the env file referenced by cmd's --env-file flag.
func loadEnvFromCmd(cmd *cobra.Command) error {
	envfile := ".env"
	explicit := false
	if cmd != nil {
		if cmd.Flags().Lookup(flagEnvFile) != nil {
			envfile, _ = cmd.Flags().GetString(flagEnvFile)
			explicit = cmd.Flags().Changed(flagEnvFile)
		}
	}
	return loadEnvFile(envfile, explicit)
}
