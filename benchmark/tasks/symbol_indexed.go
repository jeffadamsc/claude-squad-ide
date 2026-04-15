package tasks

func init() {
	Register(&SymbolFindSessionIndexed{})
	Register(&SymbolFindTypeIndexed{})
	Register(&SymbolFindUsagesIndexed{})
}

// indexHint is the context suggestion for using MCP index tools
const indexHint = `You have access to a cs-index MCP server with symbol lookup tools.
USE smart_lookup AS YOUR PRIMARY TOOL - it returns the symbol's code PLUS all functions it calls in one request.
For simple "where is X" questions, use code_search.
For "who calls X" questions, use find_references.
Fall back to Grep/Read only when the index tools don't have what you need.`

// SymbolFindSessionIndexed finds Session with index hint.
type SymbolFindSessionIndexed struct{}

func (t *SymbolFindSessionIndexed) Name() string     { return "symbol-indexed-session" }
func (t *SymbolFindSessionIndexed) Category() string { return "symbol-indexed" }
func (t *SymbolFindSessionIndexed) Prompt() string {
	return indexHint + "\n\nWhere is the Session struct defined in verve-backend? Show me the file path and line number."
}
func (t *SymbolFindSessionIndexed) Validate(output string) error { return nil }

// SymbolFindTypeIndexed finds DiffStats with index hint.
type SymbolFindTypeIndexed struct{}

func (t *SymbolFindTypeIndexed) Name() string     { return "symbol-indexed-type" }
func (t *SymbolFindTypeIndexed) Category() string { return "symbol-indexed" }
func (t *SymbolFindTypeIndexed) Prompt() string {
	return indexHint + "\n\nFind where DiffStats is defined. Show me the file and line number."
}
func (t *SymbolFindTypeIndexed) Validate(output string) error { return nil }

// SymbolFindUsagesIndexed finds usages with index hint.
type SymbolFindUsagesIndexed struct{}

func (t *SymbolFindUsagesIndexed) Name() string     { return "symbol-indexed-usages" }
func (t *SymbolFindUsagesIndexed) Category() string { return "symbol-indexed" }
func (t *SymbolFindUsagesIndexed) Prompt() string {
	return indexHint + "\n\nFind all places where Validate is called on Session. Use the symbol search to find references."
}
func (t *SymbolFindUsagesIndexed) Validate(output string) error { return nil }
