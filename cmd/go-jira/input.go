package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// stdinSentinel is the flag value that asks a text-bearing flag to read its
// content from standard input, enabling composable pipelines, e.g.
//
//	git log -1 --format=%B | go-jira run --ref -
//	cat body.md | go-jira create --project GAIA --summary x --description -
const stdinSentinel = "-"

// stdinReader is the source resolveStdin reads from; a package var so tests can
// substitute a buffer.
var stdinReader io.Reader = os.Stdin

// resolveStdin returns value unchanged unless it is the "-" sentinel, in which
// case it returns the contents of stdin with a single trailing newline trimmed.
// Only one flag per invocation can usefully read "-" (stdin is consumed once).
func resolveStdin(value string) (string, error) {
	if value != stdinSentinel {
		return value, nil
	}
	data, err := io.ReadAll(stdinReader)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}
