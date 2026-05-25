package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
)

// schemaFlag describes a single flag in machine-readable form so an agent can
// discover its name, type, default, and whether it is required without parsing
// help text.
type schemaFlag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default,omitempty"`
	Usage     string `json:"usage"`
	Required  bool   `json:"required,omitempty"`
}

// schemaCommand mirrors a cobra command (and its subtree) as a structured
// document. Path is the full invocation string so agents can copy it verbatim.
type schemaCommand struct {
	Name        string          `json:"name"`
	Path        string          `json:"path"`
	Short       string          `json:"short"`
	Long        string          `json:"long,omitempty"`
	Example     string          `json:"example,omitempty"`
	Group       string          `json:"group,omitempty"`
	Flags       []schemaFlag    `json:"flags,omitempty"`
	Subcommands []schemaCommand `json:"subcommands,omitempty"`
}

// schemaDoc is the top-level introspection payload: build metadata, the global
// (persistent) flags, and the full command tree.
type schemaDoc struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Commit      string          `json:"commit,omitempty"`
	GlobalFlags []schemaFlag    `json:"global_flags,omitempty"`
	Commands    []schemaCommand `json:"commands"`
}

// newSchemaCmd builds the `schema` command: it emits the command/flag tree as
// JSON (default) or a human-readable text outline, letting agents discover
// parameters, types, and constraints at runtime instead of scraping --help.
func newSchemaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "schema",
		Short:   "Print the command and flag schema for agent introspection",
		GroupID: groupConfig,
		Long: `Print the full command and flag schema so tools and agents can discover
available commands, their flags, types, defaults, and which flags are required
— without parsing human-oriented help text.`,
		Example: `  # Emit the machine-readable schema (default)
  go-jira schema --output json

  # Human-readable outline
  go-jira schema --output text`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSchema(cmd)
		},
	}
	addOutputFlag(cmd)
	return cmd
}

func runSchema(cmd *cobra.Command) error {
	output, _ := cmd.Flags().GetString(flagOutput)
	doc := buildSchemaDoc(cmd.Root())
	w := cmd.OutOrStdout()

	switch output {
	case outputText:
		printSchemaText(w, doc)
		return nil
	case outputJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(doc)
	default:
		return fmt.Errorf("invalid output format %q: want json or text", output)
	}
}

// buildSchemaDoc walks the command tree rooted at root and produces the
// structured introspection document.
func buildSchemaDoc(root *cobra.Command) schemaDoc {
	doc := schemaDoc{
		Name:        root.Name(),
		Version:     versionString(),
		Commit:      Commit,
		GlobalFlags: collectFlags(root.PersistentFlags()),
	}
	for _, sub := range root.Commands() {
		if sub.Hidden || sub.IsAdditionalHelpTopicCommand() {
			continue
		}
		doc.Commands = append(doc.Commands, describeCommand(sub))
	}
	return doc
}

// describeCommand renders one command and recurses into its subcommands.
func describeCommand(cmd *cobra.Command) schemaCommand {
	sc := schemaCommand{
		Name:    cmd.Name(),
		Path:    cmd.CommandPath(),
		Short:   cmd.Short,
		Long:    cmd.Long,
		Example: cmd.Example,
		Group:   cmd.GroupID,
		Flags:   collectFlags(cmd.NonInheritedFlags()),
	}
	for _, sub := range cmd.Commands() {
		if sub.Hidden || sub.IsAdditionalHelpTopicCommand() {
			continue
		}
		sc.Subcommands = append(sc.Subcommands, describeCommand(sub))
	}
	return sc
}

// collectFlags converts a pflag set into the schema representation. Required
// flags are detected via the annotation cobra sets in MarkFlagRequired.
func collectFlags(set *flag.FlagSet) []schemaFlag {
	var out []schemaFlag
	set.VisitAll(func(f *flag.Flag) {
		_, required := f.Annotations[cobra.BashCompOneRequiredFlag]
		out = append(out, schemaFlag{
			Name:      f.Name,
			Shorthand: f.Shorthand,
			Type:      f.Value.Type(),
			Default:   f.DefValue,
			Usage:     f.Usage,
			Required:  required,
		})
	})
	return out
}

// printSchemaText writes a compact human outline of the schema.
func printSchemaText(w io.Writer, doc schemaDoc) {
	fmt.Fprintf(w, "%s %s\n", doc.Name, doc.Version)
	for _, c := range doc.Commands {
		printSchemaCommandText(w, c, "  ")
	}
}

func printSchemaCommandText(w io.Writer, c schemaCommand, indent string) {
	fmt.Fprintf(w, "%s%s — %s\n", indent, c.Path, c.Short)
	for _, f := range c.Flags {
		req := ""
		if f.Required {
			req = " (required)"
		}
		fmt.Fprintf(
			w,
			"%s    --%s <%s>%s: %s\n",
			indent,
			f.Name,
			f.Type,
			req,
			strings.TrimSpace(f.Usage),
		)
	}
	for _, sub := range c.Subcommands {
		printSchemaCommandText(w, sub, indent+"  ")
	}
}
