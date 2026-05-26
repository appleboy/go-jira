package main

import (
	"fmt"
	"os"
	"time"

	jira "github.com/andygrunwald/go-jira"
	"github.com/spf13/cobra"
)

// newEpicsCmd builds the `epics` subcommand: list epics for a board via the
// Agile API. Equivalent to the Python `epics` subcommand.
func newEpicsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "epics",
		Short:   "List active epics for a board (Agile API)",
		GroupID: groupAgile,
		Example: `  # List epics for a board
  go-jira epics --board-id 10381

  # Cap the result count and emit JSON
  go-jira epics --board-id 10381 --limit 20 --output json`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEpics(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	addAuthFlags(cmd)
	addOutputFlag(cmd)
	cmd.Flags().Int(flagBoardID, 0, "Board ID, e.g. 10381 (required)")
	// Boards typically carry far more epics than sprints, so this intentionally
	// defaults higher than the sprints/boards commands (which use 10), matching
	// the reference jira.py epics default.
	cmd.Flags().Int(flagLimit, 50, "Maximum number of epics to return")
	_ = cmd.MarkFlagRequired(flagBoardID)
	return cmd
}

func runEpics(cmd *cobra.Command) error {
	config, err := loadDataConfig(cmd)
	if err != nil {
		return err
	}
	boardID, _ := cmd.Flags().GetInt(flagBoardID)
	limit, _ := cmd.Flags().GetInt(flagLimit)

	ctx, cancel := cmdContextWithTimeout(cmd, time.Minute)
	defer cancel()

	jiraClient, err := resolveJiraClient(ctx, config)
	if err != nil {
		return err
	}

	// The Python CLI always requests active epics (done=false); mirror that.
	done := false
	epics, resp, err := jiraClient.Board.GetEpicsWithContext(
		ctx, boardID, &jira.GetEpicsOptions{
			Done:          &done,
			SearchOptions: jira.SearchOptions{MaxResults: limit},
		})
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("error listing epics for board %d: %w", boardID, err)
	}

	return emitResult(config, epics, func() {
		for _, e := range epics.Values {
			fmt.Fprintf(os.Stdout, "%s\t%v\t%s\n", e.Key, e.Done, e.Name)
		}
	})
}
