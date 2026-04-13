package tasks

func init() {
	Register(&SymbolTreeSitterSession{})
}

// SymbolTreeSitterSession uses tree-sitter index tools.
type SymbolTreeSitterSession struct{}

func (t *SymbolTreeSitterSession) Name() string     { return "symbol-treesitter-session" }
func (t *SymbolTreeSitterSession) Category() string { return "symbol-treesitter" }
func (t *SymbolTreeSitterSession) Prompt() string {
	return `You have access to a cs-index MCP server with tree-sitter indexing.
Use get_symbol to find the Session struct definition. Then use find_callers
to show where it's instantiated. Show file paths and line numbers.`
}
func (t *SymbolTreeSitterSession) Validate(output string) error { return nil }
