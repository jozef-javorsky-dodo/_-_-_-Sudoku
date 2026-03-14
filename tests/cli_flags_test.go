package tests

import (
	"testing"

	"github.com/SUDOKU-ASCII/sudoku/internal/cliutil"
)

func TestMultiValueFlagSupportsRepeatAndCommaSeparatedValues(t *testing.T) {
	var flag cliutil.MultiValue

	if err := flag.Set("a.json,b.json"); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if err := flag.Set("c.json"); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	values := flag.Values()
	if len(values) != 3 {
		t.Fatalf("unexpected value count: %d", len(values))
	}
	if values[0] != "a.json" || values[1] != "b.json" || values[2] != "c.json" {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func TestMultiValueFlagUsesDefaultWhenUnset(t *testing.T) {
	var flag cliutil.MultiValue

	values := flag.Values("server.config.json")
	if len(values) != 1 || values[0] != "server.config.json" {
		t.Fatalf("unexpected defaults: %#v", values)
	}
	if flag.IsSet() {
		t.Fatalf("unset flag should not report IsSet")
	}
}

func TestMultiValueFlagReportsWhenExplicitlySet(t *testing.T) {
	var flag cliutil.MultiValue

	if err := flag.Set("a.json"); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if !flag.IsSet() {
		t.Fatalf("flag should report IsSet after Set")
	}
}
