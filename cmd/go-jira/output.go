package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Output format values for the --output flag.
const (
	outputJSON = "json"
	outputText = "text"
)

// emitResult renders a command result to stdout in the format selected by
// config.output. The default (anything other than "text") prints v as indented
// JSON, matching the Python CLI's machine-readable contract. When output is
// "text", textFn is invoked to print a concise human-readable summary instead.
//
// Diagnostics and errors are written to stderr by callers (via slog / returned
// errors); emitResult only ever writes the successful result to stdout.
func emitResult(config Config, v any, textFn func()) error {
	if config.output == outputText {
		if textFn != nil {
			textFn()
		}
		return nil
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encoding result: %w", err)
	}
	return nil
}
