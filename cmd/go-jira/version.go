package main

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"

	"github.com/spf13/cobra"
)

// buildInfo is the structured build/version payload printed by `version`.
// Commit is omitted for unstamped local builds; the remaining fields are always
// available at runtime.
type buildInfo struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// newVersionCmd builds the `version` subcommand. Unlike the single-token
// `--version` flag, this prints the full build context (commit, Go version,
// platform) that tools and agents conventionally read first. It defaults to a
// human-readable summary; --output json emits the machine-readable form.
func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "version",
		Short:   "Show version and build information",
		GroupID: groupConfig,
		Example: `  # Human-readable build info
  go-jira version

  # Machine-readable build info
  go-jira version --output json`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runVersion(cmd)
		},
	}
	// Default to text here (not addOutputFlag, which defaults to json): a human
	// running `version` should see a readable summary, while agents opt into json.
	cmd.Flags().String(flagOutput, outputText, "Output format: json|text")
	return cmd
}

func runVersion(cmd *cobra.Command) error {
	output, _ := cmd.Flags().GetString(flagOutput)
	info := buildInfo{
		Name:      cmd.Root().Name(),
		Version:   versionString(),
		Commit:    Commit,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}
	w := cmd.OutOrStdout()

	switch output {
	case outputText:
		printVersionText(w, info)
		return nil
	case outputJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	default:
		return fmt.Errorf("invalid output format %q: want json or text", output)
	}
}

func printVersionText(w io.Writer, info buildInfo) {
	fmt.Fprintf(w, "%s version %s\n", info.Name, info.Version)
	if info.Commit != "" {
		fmt.Fprintf(w, "commit:   %s\n", info.Commit)
	}
	fmt.Fprintf(w, "go:       %s\n", info.GoVersion)
	fmt.Fprintf(w, "platform: %s\n", info.Platform)
}
