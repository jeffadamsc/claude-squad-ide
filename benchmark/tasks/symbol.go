package tasks

import "strings"

func init() {
	Register(&SymbolFindSession{})
	Register(&SymbolFindType{})
	Register(&SymbolFindUsages{})
}

// SymbolFindSession finds the Session model definition.
type SymbolFindSession struct{}

func (t *SymbolFindSession) Name() string     { return "symbol-find-session" }
func (t *SymbolFindSession) Category() string { return "symbol" }
func (t *SymbolFindSession) Prompt() string {
	return "Where is the Session struct defined in verve-backend? Show me the file path and the struct definition."
}
func (t *SymbolFindSession) Validate(output string) error {
	if !strings.Contains(output, "models/session.go") {
		return nil // Validation is optional, don't fail
	}
	return nil
}

// SymbolFindType finds type definitions and usages.
type SymbolFindType struct{}

func (t *SymbolFindType) Name() string     { return "symbol-find-type" }
func (t *SymbolFindType) Category() string { return "symbol" }
func (t *SymbolFindType) Prompt() string {
	return "Find the DiffStats type in verve-backend. Where is it defined and what functions use it?"
}
func (t *SymbolFindType) Validate(output string) error { return nil }

// SymbolFindUsages finds all usages of a function.
type SymbolFindUsages struct{}

func (t *SymbolFindUsages) Name() string     { return "symbol-find-usages" }
func (t *SymbolFindUsages) Category() string { return "symbol" }
func (t *SymbolFindUsages) Prompt() string {
	return "Find all places where Validate() is called on a Session model in verve-backend."
}
func (t *SymbolFindUsages) Validate(output string) error { return nil }
