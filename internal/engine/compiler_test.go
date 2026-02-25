package engine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vinaysrao1/nest/internal/domain"
	"go.starlark.net/starlark"
)

// loadTestdata reads a .star file from the testdata directory relative to this package.
func loadTestdata(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("loadTestdata: read %s: %v", name, err)
	}
	return string(data)
}

// assertCompileError asserts that err is a *domain.CompileError whose Message contains wantMsg.
func assertCompileError(t *testing.T, err error, wantMsg string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected CompileError containing %q, got nil", wantMsg)
	}
	var ce *domain.CompileError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T, want *domain.CompileError; err = %v", err, err)
	}
	if ce.Message == "" {
		t.Error("CompileError.Message is empty")
	}
	if wantMsg != "" && !strings.Contains(ce.Message, wantMsg) {
		t.Errorf("CompileError.Message = %q, want it to contain %q", ce.Message, wantMsg)
	}
}

// TestCompileRule_ValidRule verifies a well-formed rule compiles with correct metadata.
func TestCompileRule_ValidRule(t *testing.T) {
	t.Parallel()
	c := &Compiler{}
	src := loadTestdata(t, "valid_rule.star")

	rule, err := c.CompileRule(src, "valid_rule.star")
	if err != nil {
		t.Fatalf("CompileRule returned unexpected error: %v", err)
	}
	if rule.ID != "test-block-spam" {
		t.Errorf("rule.ID = %q, want %q", rule.ID, "test-block-spam")
	}
	if len(rule.EventTypes) != 1 || rule.EventTypes[0] != "content" {
		t.Errorf("rule.EventTypes = %v, want [content]", rule.EventTypes)
	}
	if rule.Priority != 100 {
		t.Errorf("rule.Priority = %d, want 100", rule.Priority)
	}
	if rule.Program == nil {
		t.Error("rule.Program is nil, want non-nil")
	}
	if rule.Source != src {
		t.Error("rule.Source does not match input source")
	}
}

// TestCompileRule_WildcardRule verifies a wildcard event_types=["*"] rule compiles correctly.
func TestCompileRule_WildcardRule(t *testing.T) {
	t.Parallel()
	c := &Compiler{}
	src := loadTestdata(t, "wildcard_rule.star")

	rule, err := c.CompileRule(src, "wildcard_rule.star")
	if err != nil {
		t.Fatalf("CompileRule returned unexpected error: %v", err)
	}
	if len(rule.EventTypes) != 1 || rule.EventTypes[0] != "*" {
		t.Errorf("rule.EventTypes = %v, want [*]", rule.EventTypes)
	}
}

// TestCompileRule_MissingEvaluate verifies a rule without evaluate() returns CompileError.
func TestCompileRule_MissingEvaluate(t *testing.T) {
	t.Parallel()
	c := &Compiler{}
	src := loadTestdata(t, "missing_evaluate.star")
	_, err := c.CompileRule(src, "missing_evaluate.star")
	assertCompileError(t, err, "missing required function: evaluate")
}

// TestCompileRule_MissingRuleID verifies a rule without rule_id returns CompileError.
func TestCompileRule_MissingRuleID(t *testing.T) {
	t.Parallel()
	c := &Compiler{}
	src := loadTestdata(t, "missing_rule_id.star")
	_, err := c.CompileRule(src, "missing_rule_id.star")
	assertCompileError(t, err, "missing required global: rule_id")
}

// TestCompileRule_MissingEventTypes verifies a rule without event_types returns CompileError.
func TestCompileRule_MissingEventTypes(t *testing.T) {
	t.Parallel()
	c := &Compiler{}
	src := loadTestdata(t, "missing_event_types.star")
	_, err := c.CompileRule(src, "missing_event_types.star")
	assertCompileError(t, err, "missing required global: event_types")
}

// TestCompileRule_MissingPriority verifies a rule without priority returns CompileError.
func TestCompileRule_MissingPriority(t *testing.T) {
	t.Parallel()
	c := &Compiler{}
	src := loadTestdata(t, "missing_priority.star")
	_, err := c.CompileRule(src, "missing_priority.star")
	assertCompileError(t, err, "missing required global: priority")
}

// TestCompileRule_MixedWildcard verifies that mixing "*" with specific event types is rejected.
func TestCompileRule_MixedWildcard(t *testing.T) {
	t.Parallel()
	c := &Compiler{}
	src := loadTestdata(t, "mixed_wildcard.star")
	_, err := c.CompileRule(src, "mixed_wildcard.star")
	assertCompileError(t, err, `event_types cannot mix "*" with specific types`)
}

// TestCompileRule_SyntaxError verifies that a syntax error produces a CompileError with line info.
func TestCompileRule_SyntaxError(t *testing.T) {
	t.Parallel()
	c := &Compiler{}
	src := loadTestdata(t, "syntax_error.star")

	_, err := c.CompileRule(src, "syntax_error.star")
	if err == nil {
		t.Fatal("CompileRule: expected error for syntax_error.star, got nil")
	}
	var ce *domain.CompileError
	if !errors.As(err, &ce) {
		t.Fatalf("CompileRule: error type = %T, want *domain.CompileError", err)
	}
	if ce.Line == 0 {
		t.Error("CompileError.Line = 0, want non-zero for syntax errors with position info")
	}
}

// TestCompileRule_ProgramReusability verifies the compiled Program can be Init'd multiple times
// independently, confirming the Program is reusable for repeated rule evaluations.
func TestCompileRule_ProgramReusability(t *testing.T) {
	t.Parallel()
	c := &Compiler{}
	src := loadTestdata(t, "valid_rule.star")

	rule, err := c.CompileRule(src, "valid_rule.star")
	if err != nil {
		t.Fatalf("CompileRule: %v", err)
	}

	for i := range 2 {
		thread := &starlark.Thread{Name: "test-reuse"}
		globals, initErr := rule.Program.Init(thread, predeclaredNames)
		if initErr != nil {
			t.Errorf("Program.Init call %d failed: %v", i, initErr)
			continue
		}
		if _, ok := globals["evaluate"]; !ok {
			t.Errorf("Program.Init call %d: globals missing 'evaluate'", i)
		}
	}
}
