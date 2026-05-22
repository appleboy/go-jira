package main

import (
	"context"
	"fmt"
	"os"
	"time"

	jira "github.com/andygrunwald/go-jira"
	"github.com/spf13/cobra"
)

// newBoardsCmd builds the `boards` subcommand: discover boards for a project
// via the Agile API. Equivalent to the Python `boards` subcommand.
func newBoardsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "boards",
		Short:        "Discover boards for a project (Agile API)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBoards(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	addAuthFlags(cmd)
	addOutputFlag(cmd)
	cmd.Flags().String(flagProject, "", "Project key, e.g. GAIA (required)")
	cmd.Flags().String(flagBoardType, "", "Filter by board type: scrum|kanban")
	cmd.Flags().Int(flagLimit, 10, "Maximum number of boards to return")
	_ = cmd.MarkFlagRequired(flagProject)
	return cmd
}

func runBoards(cmd *cobra.Command) error {
	config, err := loadDataConfig(cmd)
	if err != nil {
		return err
	}
	project, _ := cmd.Flags().GetString(flagProject)
	boardType, _ := cmd.Flags().GetString(flagBoardType)
	limit, _ := cmd.Flags().GetInt(flagLimit)

	ctx, cancel := context.WithTimeout(cmdContext(cmd), time.Minute)
	defer cancel()

	jiraClient, err := resolveJiraClient(ctx, config)
	if err != nil {
		return err
	}

	boards, resp, err := jiraClient.Board.GetAllBoardsWithContext(ctx, &jira.BoardListOptions{
		ProjectKeyOrID: project,
		BoardType:      boardType,
		SearchOptions:  jira.SearchOptions{MaxResults: limit},
	})
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("error listing boards for project %s: %w", project, err)
	}

	return emitResult(config, boards, func() {
		for _, b := range boards.Values {
			fmt.Fprintf(os.Stdout, "%d\t%s\t%s\n", b.ID, b.Type, b.Name)
		}
	})
}
