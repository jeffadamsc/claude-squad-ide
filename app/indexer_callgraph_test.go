package app

import "testing"

func TestCallGraphFindCallers(t *testing.T) {
	cg := NewCallGraph()

	cg.AddReference(Reference{
		Symbol: "helper",
		Caller: "main",
		File:   "main.go",
		Line:   10,
		Kind:   "call",
	})
	cg.AddReference(Reference{
		Symbol: "helper",
		Caller: "process",
		File:   "process.go",
		Line:   20,
		Kind:   "call",
	})

	callers := cg.FindCallers("helper")
	if len(callers) != 2 {
		t.Errorf("FindCallers(helper) = %d, want 2", len(callers))
	}
}

func TestCallGraphFindCallees(t *testing.T) {
	cg := NewCallGraph()

	cg.AddReference(Reference{
		Symbol: "helper",
		Caller: "main",
		File:   "main.go",
		Line:   10,
		Kind:   "call",
	})
	cg.AddReference(Reference{
		Symbol: "process",
		Caller: "main",
		File:   "main.go",
		Line:   15,
		Kind:   "call",
	})

	callees := cg.FindCallees("main")
	if len(callees) != 2 {
		t.Errorf("FindCallees(main) = %d, want 2", len(callees))
	}
}
