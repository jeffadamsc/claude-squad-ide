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
Use smart_lookup to find and understand the Session struct - this will return the struct
definition plus any methods or functions it uses. Show file paths and line numbers.`
}
func (t *SymbolTreeSitterSession) Validate(output string) error { return nil }
