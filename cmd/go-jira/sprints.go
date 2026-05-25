package main

import (
	"context"
	"fmt"
	"os"
	"time"

	jira "github.com/andygrunwald/go-jira"
	"github.com/spf13/cobra"
)

// newSprintsCmd builds the `sprints` subcommand: list sprints for a board via
// the Agile API. Equivalent to the Python `sprints` subcommand.
func newSprintsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sprints",
		Short:   "List sprints for a board (Agile API)",
		GroupID: groupAgile,
		Example: `  # List active sprints for a board
  go-jira sprints --board-id 10381

  # List closed sprints, JSON output
  go-jira sprints --board-id 10381 --state closed --output json`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSprints(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	addAuthFlags(cmd)
	addOutputFlag(cmd)
	cmd.Flags().Int(flagBoardID, 0, "Board ID, e.g. 10381 (required)")
	cmd.Flags().String(flagState, "active", "Sprint state filter: active|closed|future")
	cmd.Flags().Int(flagLimit, 10, "Maximum number of sprints to return")
	_ = cmd.MarkFlagRequired(flagBoardID)
	return cmd
}

func runSprints(cmd *cobra.Command) error {
	config, err := loadDataConfig(cmd)
	if err != nil {
		return err
	}
	boardID, _ := cmd.Flags().GetInt(flagBoardID)
	state, _ := cmd.Flags().GetString(flagState)
	limit, _ := cmd.Flags().GetInt(flagLimit)

	ctx, cancel := context.WithTimeout(cmdContext(cmd), time.Minute)
	defer cancel()

	jiraClient, err := resolveJiraClient(ctx, config)
	if err != nil {
		return err
	}

	sprints, resp, err := jiraClient.Board.GetAllSprintsWithOptionsWithContext(
		ctx, boardID, &jira.GetAllSprintsOptions{
			State:         state,
			SearchOptions: jira.SearchOptions{MaxResults: limit},
		})
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("error listing sprints for board %d: %w", boardID, err)
	}

	return emitResult(config, sprints, func() {
		for _, sp := range sprints.Values {
			fmt.Fprintf(os.Stdout, "%d\t%s\t%s\n", sp.ID, sp.State, sp.Name)
		}
	})
}
