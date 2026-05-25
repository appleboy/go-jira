package main

import (
	"context"
	"fmt"
	"os"
	"time"

	jira "github.com/andygrunwald/go-jira"
	"github.com/spf13/cobra"
)

// newLinkCmd builds the `link` subcommand: create an issue link between two
// tickets. Equivalent to the Python `link` subcommand.
func newLinkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "link",
		Short:   "Create an issue link between two tickets",
		GroupID: groupIssues,
		Example: `  # Link two issues with the default "Relates" type
  go-jira link --from GAIA-1 --to GAIA-2

  # Use a specific link type
  go-jira link --from GAIA-1 --to GAIA-2 --link-type Blocks`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLink(cmd)
		},
	}
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	addAuthFlags(cmd)
	addOutputFlag(cmd)
	cmd.Flags().String(flagFrom, "", "Inward issue key (required)")
	cmd.Flags().String(flagTo, "", "Outward issue key (required)")
	cmd.Flags().String(flagLinkType, "Relates", "Link type name")
	_ = cmd.MarkFlagRequired(flagFrom)
	_ = cmd.MarkFlagRequired(flagTo)
	return cmd
}

func runLink(cmd *cobra.Command) error {
	config, err := loadDataConfig(cmd)
	if err != nil {
		return err
	}
	from, _ := cmd.Flags().GetString(flagFrom)
	to, _ := cmd.Flags().GetString(flagTo)
	linkType, _ := cmd.Flags().GetString(flagLinkType)

	ctx, cancel := context.WithTimeout(cmdContext(cmd), time.Minute)
	defer cancel()

	jiraClient, err := resolveJiraClient(ctx, config)
	if err != nil {
		return err
	}

	resp, err := jiraClient.Issue.AddLinkWithContext(ctx, &jira.IssueLink{
		Type:         jira.IssueLinkType{Name: linkType},
		InwardIssue:  &jira.Issue{Key: from},
		OutwardIssue: &jira.Issue{Key: to},
	})
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("error linking %s -> %s: %w", from, to, err)
	}

	result := map[string]string{statusKey: "linked", "from": from, "to": to}
	return emitResult(config, result, func() {
		fmt.Fprintf(os.Stdout, "linked %s -> %s (%s)\n", from, to, linkType)
	})
}
