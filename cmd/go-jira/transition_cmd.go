package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	jira "github.com/andygrunwald/go-jira"
	"github.com/spf13/cobra"
)

// transitionOutput is the stable representation shared by the standalone
// transition list and execute commands. Jira's Status contains substantially
// more data than callers need here, so To is the destination status name.
type transitionOutput struct {
	ID     string                          `json:"id"`
	Name   string                          `json:"name"`
	To     string                          `json:"to"`
	Fields map[string]jira.TransitionField `json:"fields"`
}

type transitionListResult struct {
	Key         string             `json:"key"`
	Transitions []transitionOutput `json:"transitions"`
}

type transitionExecuteResult struct {
	Status string `json:"status"`
	Key    string `json:"key"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	To     string `json:"to"`
}

// newTransitionCmd builds the `transition` command group for discovering and
// executing the transitions currently available for one issue.
func newTransitionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "transition",
		Short:   "List or execute transitions for a Jira issue",
		GroupID: groupIssues,
		Long: `List the transitions currently available for a Jira issue, or execute one
after validating its exact ID or case-insensitive name against a fresh list.`,
		Example: `  # List transitions available for an issue
  go-jira transition list --key GAIA-123

  # Execute by transition name or ID
  go-jira transition execute --key GAIA-123 --transition Done
  go-jira transition execute --key GAIA-123 --transition 31`,
		SilenceUsage: true,
	}
	cmd.AddCommand(newTransitionListCmd(), newTransitionExecuteCmd())
	return cmd
}

func newTransitionListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List transitions available for a Jira issue",
		Example: `  # Machine-readable output (default)
  go-jira transition list --key GAIA-123 --output json

  # Tab-separated ID, name, and destination status
  go-jira transition list --key GAIA-123 --output text`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTransitionList(cmd)
		},
	}
	addTransitionCommandFlags(cmd)
	cmd.Flags().String(flagKey, "", "Issue key, e.g. GAIA-123 (required)")
	_ = cmd.MarkFlagRequired(flagKey)
	return cmd
}

func newTransitionExecuteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute",
		Short: "Execute an available transition for a Jira issue",
		Long: `Execute a transition after fetching the choices currently available for the
issue. An exact transition ID takes precedence; otherwise the selector must
match exactly one transition name, case-insensitively.`,
		Example: `  # Execute by case-insensitive exact name
  go-jira transition execute --key GAIA-123 --transition Done

  # Execute by exact transition ID and set a resolution
  go-jira transition execute --key GAIA-123 --transition 31 --resolution Fixed`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTransitionExecute(cmd)
		},
	}
	addTransitionCommandFlags(cmd)
	cmd.Flags().String(flagKey, "", "Issue key, e.g. GAIA-123 (required)")
	cmd.Flags().String(flagTransition, "", "Transition ID or exact name (required)")
	cmd.Flags().String(flagResolution, "", "Resolution name to set during the transition")
	_ = cmd.MarkFlagRequired(flagKey)
	_ = cmd.MarkFlagRequired(flagTransition)
	return cmd
}

func addTransitionCommandFlags(cmd *cobra.Command) {
	addCommonFlags(cmd)
	addOAuthFlags(cmd)
	addAuthFlags(cmd)
	addOutputFlag(cmd)
}

func runTransitionList(cmd *cobra.Command) error {
	config, err := loadDataConfig(cmd)
	if err != nil {
		return err
	}
	key, _ := cmd.Flags().GetString(flagKey)

	ctx, cancel := cmdContextWithTimeout(cmd, time.Minute)
	defer cancel()

	jiraClient, err := resolveJiraClient(ctx, config)
	if err != nil {
		return err
	}

	transitions, err := getAvailableTransitions(ctx, jiraClient, key)
	if err != nil {
		return err
	}
	result := transitionListResult{
		Key:         key,
		Transitions: transitionOutputs(transitions),
	}
	return emitResult(config, result, func() {
		for _, transition := range result.Transitions {
			fmt.Fprintf(os.Stdout, "%s\t%s\t%s\n",
				transition.ID, transition.Name, transition.To)
		}
	})
}

func runTransitionExecute(cmd *cobra.Command) error {
	config, err := loadDataConfig(cmd)
	if err != nil {
		return err
	}
	key, _ := cmd.Flags().GetString(flagKey)
	selector, _ := cmd.Flags().GetString(flagTransition)
	resolution, _ := cmd.Flags().GetString(flagResolution)

	ctx, cancel := cmdContextWithTimeout(cmd, time.Minute)
	defer cancel()

	jiraClient, err := resolveJiraClient(ctx, config)
	if err != nil {
		return err
	}

	// Always validate the selector against a fresh list. Jira only returns
	// transitions available for this issue's current state and the current user.
	transitions, err := getAvailableTransitions(ctx, jiraClient, key)
	if err != nil {
		return err
	}
	selected, err := resolveAvailableTransition(selector, transitions)
	if err != nil {
		return fmt.Errorf("issue %s: %w", key, err)
	}

	var resp *jira.Response
	if resolution == "" {
		resp, err = jiraClient.Issue.DoTransitionWithContext(ctx, key, selected.ID)
	} else {
		resolutionID, lookupErr := getResolutionID(ctx, jiraClient, resolution)
		if lookupErr != nil {
			return fmt.Errorf("error resolving resolution %q for issue %s: %w",
				resolution, key, lookupErr)
		}
		if resolutionID == "" {
			return fmt.Errorf("resolution %q not found for issue %s", resolution, key)
		}
		resp, err = jiraClient.Issue.DoTransitionPayloadWithContext(
			ctx,
			&jira.TransitionPayloadInput{
				TicketID:     key,
				TransitionID: selected.ID,
				ResolutionID: &resolutionID,
			},
		)
	}
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("error transitioning issue %s via %s (%s): %w",
			key, selected.ID, selected.Name, err)
	}
	if resp == nil {
		return fmt.Errorf("error transitioning issue %s via %s (%s): no response returned",
			key, selected.ID, selected.Name)
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("error transitioning issue %s via %s (%s): unexpected status: %s",
			key, selected.ID, selected.Name, resp.Status)
	}

	result := transitionExecuteResult{
		Status: "transitioned",
		Key:    key,
		ID:     selected.ID,
		Name:   selected.Name,
		To:     selected.To.Name,
	}
	return emitResult(config, result, func() {
		fmt.Fprintf(os.Stdout, "transitioned %s via %s (%s) -> %s\n",
			result.Key, result.ID, result.Name, result.To)
	})
}

// getAvailableTransitions reads the choices Jira currently exposes for the
// issue. The context in the returned error is intentionally added here so both
// list and execute produce the same actionable failure.
func getAvailableTransitions(
	ctx context.Context,
	jiraClient *jira.Client,
	key string,
) ([]jira.Transition, error) {
	transitions, resp, err := jiraClient.Issue.GetTransitionsWithContext(ctx, key)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("error listing transitions for issue %s: %w", key, err)
	}
	if transitions == nil {
		transitions = []jira.Transition{}
	}
	return transitions, nil
}

// resolveAvailableTransition selects an exact ID before considering names. A
// duplicate case-insensitive name is refused so callers cannot accidentally
// move an issue through an unintended workflow edge.
func resolveAvailableTransition(
	selector string,
	transitions []jira.Transition,
) (jira.Transition, error) {
	for _, transition := range transitions {
		if transition.ID == selector {
			return transition, nil
		}
	}

	matches := make([]jira.Transition, 0, 1)
	for _, transition := range transitions {
		if strings.EqualFold(transition.Name, selector) {
			matches = append(matches, transition)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return jira.Transition{}, fmt.Errorf(
			"transition %q is unavailable; available transitions: %s",
			selector,
			transitionChoices(transitions),
		)
	default:
		return jira.Transition{}, fmt.Errorf(
			"transition name %q is ambiguous; matching transitions: %s; use an exact transition ID",
			selector,
			transitionChoices(matches),
		)
	}
}

func transitionChoices(transitions []jira.Transition) string {
	if len(transitions) == 0 {
		return "none"
	}
	choices := make([]string, 0, len(transitions))
	for _, transition := range transitions {
		choices = append(choices, transition.ID+"/"+transition.Name)
	}
	return strings.Join(choices, ", ")
}

func transitionOutputs(transitions []jira.Transition) []transitionOutput {
	output := make([]transitionOutput, 0, len(transitions))
	for _, transition := range transitions {
		output = append(output, transitionOutput{
			ID:     transition.ID,
			Name:   transition.Name,
			To:     transition.To.Name,
			Fields: transition.Fields,
		})
	}
	return output
}
