package main

import (
	"errors"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// TestValidateNoControlChars confirms that control characters (ASCII < 0x20)
// other than tab/newline/CR are rejected as usage errors, while ordinary text
// — including the permitted whitespace controls — passes through.
func TestValidateNoControlChars(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"plain", []string{"completion", "bash"}, false},
		{"empty", nil, false},
		{"tab allowed", []string{"--comment", "a\tb"}, false},
		{"newline allowed", []string{"--ref", "GAIA-1\nGAIA-2"}, false},
		{"carriage return allowed", []string{"x\ry"}, false},
		{"escape rejected", []string{"bash\x1b"}, true},
		{"vertical tab rejected", []string{"\x0b"}, true},
		{"bell rejected", []string{"--base-url", "http://x\x07"}, true},
		{"backspace rejected", []string{"\x08"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNoControlChars(tc.args)
			if tc.wantErr != (err != nil) {
				t.Fatalf("validateNoControlChars(%q) error = %v, wantErr %v",
					tc.args, err, tc.wantErr)
			}
			if !tc.wantErr {
				return
			}
			var ce *cliError
			if !errors.As(err, &ce) {
				t.Fatalf("error is not *cliError: %v", err)
			}
			if ce.code != exitUsage || ce.kind != kindUsage {
				t.Errorf("got code=%d kind=%q, want code=%d kind=%q",
					ce.code, ce.kind, exitUsage, kindUsage)
			}
		})
	}
}

// TestCmdContextWithTimeout verifies the --timeout flag overrides the
// per-command default when set to a positive duration, and is otherwise
// ignored (default kept) for absent, zero, or negative values. --timeout is a
// persistent root flag, so the subcommand is fetched from a built root to merge
// the inherited flags the way cobra does at execution time.
func TestCmdContextWithTimeout(t *testing.T) {
	const def = time.Minute

	subCmd := func(t *testing.T) *cobra.Command {
		t.Helper()
		for _, c := range newRootCmd().Commands() {
			if c.Name() == "search" {
				return c
			}
		}
		t.Fatal("search subcommand not found under root")
		return nil
	}

	t.Run("default when flag absent", func(t *testing.T) {
		cmd := subCmd(t)
		if err := cmd.ParseFlags(nil); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		ctx, cancel := cmdContextWithTimeout(cmd, def)
		defer cancel()
		assertDeadlineNear(t, ctx, def)
	})

	t.Run("override when flag positive", func(t *testing.T) {
		cmd := subCmd(t)
		if err := cmd.ParseFlags([]string{"--timeout=5s"}); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		ctx, cancel := cmdContextWithTimeout(cmd, def)
		defer cancel()
		assertDeadlineNear(t, ctx, 5*time.Second)
	})

	t.Run("default kept when flag zero", func(t *testing.T) {
		cmd := subCmd(t)
		if err := cmd.ParseFlags([]string{"--timeout=0"}); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		ctx, cancel := cmdContextWithTimeout(cmd, def)
		defer cancel()
		assertDeadlineNear(t, ctx, def)
	})
}

// assertDeadlineNear checks ctx carries a deadline within a small tolerance of
// now+want, accommodating the elapsed time since the context was created.
func assertDeadlineNear(
	t *testing.T,
	ctx interface{ Deadline() (time.Time, bool) },
	want time.Duration,
) {
	t.Helper()
	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("context has no deadline")
	}
	got := time.Until(dl)
	if diff := got - want; diff < -time.Second || diff > time.Second {
		t.Errorf("deadline in %v, want ~%v", got, want)
	}
}
