package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

// runSchemaCapture executes `schema` with the given output flag against a fresh
// root command, capturing stdout.
func runSchemaCapture(t *testing.T, output string) string {
	t.Helper()
	root := newRootCmd()
	root.SetArgs([]string{"schema", "--output", output})
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("schema --output %s: %v", output, err)
	}
	return out.String()
}

// TestSchemaJSONIncludesCommandsAndRequiredFlags verifies the schema lists
// commands with their flags, marks required flags, and reports the build
// version — enough for an agent to discover the interface.
func TestSchemaJSONIncludesCommandsAndRequiredFlags(t *testing.T) {
	doc := buildSchemaDoc(newRootCmd())

	if doc.Version != versionString() {
		t.Fatalf("version = %q, want %q", doc.Version, versionString())
	}

	search := findCommand(doc.Commands, "search")
	if search == nil {
		t.Fatal("search command missing from schema")
	}
	jql := findFlag(search.Flags, flagJQL)
	if jql == nil {
		t.Fatalf("--%s missing from search schema", flagJQL)
	}
	if !jql.Required {
		t.Errorf("--%s should be marked required", flagJQL)
	}

	// Nested command tree: token -> status.
	token := findCommand(doc.Commands, "token")
	if token == nil || findCommand(token.Subcommands, "status") == nil {
		t.Fatal("expected token.status in schema subcommand tree")
	}

	// Nested issue-transition command tree, including the required selector
	// flags agents need to discover before attempting a state-changing call.
	transition := findCommand(doc.Commands, "transition")
	if transition == nil {
		t.Fatal("transition command missing from schema")
	}
	if transition.Group != groupIssues {
		t.Errorf("transition group = %q, want %q", transition.Group, groupIssues)
	}
	list := findCommand(transition.Subcommands, "list")
	if list == nil {
		t.Fatal("expected transition.list in schema subcommand tree")
	}
	listKey := findFlag(list.Flags, flagKey)
	if listKey == nil || !listKey.Required {
		t.Fatalf("transition.list --%s should be present and required", flagKey)
	}

	execute := findCommand(transition.Subcommands, "execute")
	if execute == nil {
		t.Fatal("expected transition.execute in schema subcommand tree")
	}
	for _, name := range []string{flagKey, flagTransition} {
		flag := findFlag(execute.Flags, name)
		if flag == nil || !flag.Required {
			t.Errorf("transition.execute --%s should be present and required", name)
		}
	}
	resolution := findFlag(execute.Flags, flagResolution)
	if resolution == nil {
		t.Fatalf("transition.execute --%s should be present", flagResolution)
	}
	if resolution.Required {
		t.Errorf("transition.execute --%s should remain optional", flagResolution)
	}
}

// TestSchemaJSONIsValidJSON ensures the rendered JSON output parses cleanly.
func TestSchemaJSONIsValidJSON(t *testing.T) {
	out := runSchemaCapture(t, outputJSON)
	var doc schemaDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("schema JSON did not parse: %v", err)
	}
	if len(doc.Commands) == 0 {
		t.Fatal("schema reported no commands")
	}
}

func findCommand(cmds []schemaCommand, name string) *schemaCommand {
	for i := range cmds {
		if cmds[i].Name == name {
			return &cmds[i]
		}
	}
	return nil
}

func findFlag(flags []schemaFlag, name string) *schemaFlag {
	for i := range flags {
		if flags[i].Name == name {
			return &flags[i]
		}
	}
	return nil
}
