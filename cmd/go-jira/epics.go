package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// epicValue is the subset of fields we surface from each entry in the Agile
// board-epic response. The endpoint returns the standard paginated envelope
// ({maxResults, startAt, isLast, values:[...]}); unknown fields are ignored by
// encoding/json so this stays resilient across Jira Server/DC versions.
type epicValue struct {
	ID      int    `json:"id"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Done    bool   `json:"done"`
}

// epicsList is the paginated envelope returned by
// GET /rest/agile/1.0/board/{boardID}/epic.
type epicsList struct {
	MaxResults int         `json:"maxResults"`
	StartAt    int         `json:"startAt"`
	IsLast     bool        `json:"isLast"`
	Values     []epicValue `json:"values"`
}

// newEpicsCmd builds the `epics` subcommand: list epics for a board via the
// Agile API. Equivalent to the Python `epics` subcommand.
//
// Unlike `sprints` and `boards`, the vendored go-jira library's BoardService
// has no epic-listing method, so this command issues the request through the
// client's generic NewRequestWithContext + Do path rather than a typed service
// call.
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

	ctx, cancel := context.WithTimeout(cmdContext(cmd), time.Minute)
	defer cancel()

	jiraClient, err := resolveJiraClient(ctx, config)
	if err != nil {
		return err
	}

	// The Python CLI always requests active epics (done=false); mirror that.
	qs := url.Values{}
	qs.Set("done", "false")
	qs.Set("maxResults", fmt.Sprintf("%d", limit))
	endpoint := fmt.Sprintf("rest/agile/1.0/board/%d/epic?%s", boardID, qs.Encode())

	req, err := jiraClient.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return fmt.Errorf("error building epics request for board %d: %w", boardID, err)
	}

	var epics epicsList
	resp, err := jiraClient.Do(req, &epics)
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
