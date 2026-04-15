package app

import (
	"context"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// LangConfig holds parser and extraction logic for a language.
type LangConfig struct {
	Language    *sitter.Language
	Extensions  []string
	SymbolKinds map[string]string // node type -> symbol kind
	RefKinds    map[string]string // node type -> reference kind
}

var langConfigs = map[string]*LangConfig{
	"go": {
		Language:   golang.GetLanguage(),
		Extensions: []string{".go"},
		SymbolKinds: map[string]string{
			"function_declaration": "function",
			"method_declaration":   "method",
			"type_declaration":     "type",
			"type_spec":            "type",
			"const_spec":           "constant",
			"var_spec":             "variable",
		},
		RefKinds: map[string]string{
			"call_expression":     "call",
			"selector_expression": "reference",
		},
	},
	"typescript": {
		Language:   typescript.GetLanguage(),
		Extensions: []string{".ts", ".tsx"},
		SymbolKinds: map[string]string{
			"function_declaration":   "function",
			"method_definition":      "method",
			"class_declaration":      "class",
			"interface_declaration":  "interface",
			"type_alias_declaration": "type",
			"variable_declarator":    "variable",
			"lexical_declaration":    "variable",
		},
		RefKinds: map[string]string{
			"call_expression":  "call",
			"member_expression": "reference",
		},
	},
	"python": {
		Language:   python.GetLanguage(),
		Extensions: []string{".py"},
		SymbolKinds: map[string]string{
			"function_definition": "function",
			"class_definition":    "class",
			"assignment":          "variable",
		},
		RefKinds: map[string]string{
			"call":      "call",
			"attribute": "reference",
		},
	},
	"javascript": {
		Language:   javascript.GetLanguage(),
		Extensions: []string{".js", ".jsx", ".mjs"},
		SymbolKinds: map[string]string{
			"function_declaration": "function",
			"method_definition":    "method",
			"class_declaration":    "class",
			"variable_declarator":  "variable",
			"arrow_function":       "function",
		},
		RefKinds: map[string]string{
			"call_expression":   "call",
			"member_expression": "reference",
		},
	},
	"java": {
		Language:   java.GetLanguage(),
		Extensions: []string{".java"},
		SymbolKinds: map[string]string{
			"method_declaration":       "method",
			"class_declaration":        "class",
			"interface_declaration":    "interface",
			"field_declaration":        "variable",
			"constructor_declaration":  "constructor",
		},
		RefKinds: map[string]string{
			"method_invocation": "call",
		},
	},
	"c": {
		Language:   c.GetLanguage(),
		Extensions: []string{".c", ".h"},
		SymbolKinds: map[string]string{
			"function_definition": "function",
			"declaration":         "variable",
			"struct_specifier":    "type",
			"enum_specifier":      "type",
			"type_definition":     "type",
		},
		RefKinds: map[string]string{
			"call_expression": "call",
		},
	},
	"cpp": {
		Language:   cpp.GetLanguage(),
		Extensions: []string{".cpp", ".cc", ".cxx", ".hpp", ".hxx"},
		SymbolKinds: map[string]string{
			"function_definition":  "function",
			"class_specifier":      "class",
			"struct_specifier":     "type",
			"namespace_definition": "namespace",
			"template_declaration": "template",
		},
		RefKinds: map[string]string{
			"call_expression": "call",
		},
	},
	"rust": {
		Language:   rust.GetLanguage(),
		Extensions: []string{".rs"},
		SymbolKinds: map[string]string{
			"function_item": "function",
			"impl_item":     "impl",
			"struct_item":   "type",
			"enum_item":     "type",
			"trait_item":    "trait",
			"mod_item":      "module",
			"const_item":    "constant",
			"static_item":   "variable",
		},
		RefKinds: map[string]string{
			"call_expression":  "call",
			"macro_invocation": "call",
		},
	},
	"ruby": {
		Language:   ruby.GetLanguage(),
		Extensions: []string{".rb"},
		SymbolKinds: map[string]string{
			"method":           "method",
			"singleton_method": "method",
			"class":            "class",
			"module":           "module",
		},
		RefKinds: map[string]string{
			"call":        "call",
			"method_call": "call",
		},
	},
	"bash": {
		Language:   bash.GetLanguage(),
		Extensions: []string{".sh", ".bash"},
		SymbolKinds: map[string]string{
			"function_definition": "function",
		},
		RefKinds: map[string]string{
			"command": "call",
		},
	},
	"csharp": {
		Language:   csharp.GetLanguage(),
		Extensions: []string{".cs"},
		SymbolKinds: map[string]string{
			"method_declaration":    "method",
			"class_declaration":     "class",
			"interface_declaration": "interface",
			"struct_declaration":    "type",
			"property_declaration":  "property",
		},
		RefKinds: map[string]string{
			"invocation_expression": "call",
		},
	},
}

// getLangConfig returns the config for a file extension.
func getLangConfig(filename string) *LangConfig {
	ext := strings.ToLower(filepath.Ext(filename))
	for _, cfg := range langConfigs {
		for _, e := range cfg.Extensions {
			if e == ext {
				return cfg
			}
		}
	}
	return nil
}

// extractSymbols parses a file and extracts symbols and references.
func extractSymbols(filename string, content []byte) ([]Symbol, []Reference, error) {
	cfg := getLangConfig(filename)
	if cfg == nil {
		return nil, nil, nil // unsupported language
	}

	parser := sitter.NewParser()
	parser.SetLanguage(cfg.Language)

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, nil, err
	}
	defer tree.Close()

	var symbols []Symbol
	var refs []Reference

	lang := langNameFromConfig(cfg)
	walkNode(tree.RootNode(), cfg, filename, lang, content, "", &symbols, &refs)

	return symbols, refs, nil
}

func langNameFromConfig(cfg *LangConfig) string {
	for name, c := range langConfigs {
		if c == cfg {
			return name
		}
	}
	return "unknown"
}

// walkNode recursively walks the AST extracting symbols and references.
func walkNode(node *sitter.Node, cfg *LangConfig, file, lang string, content []byte, scope string, symbols *[]Symbol, refs *[]Reference) {
	nodeType := node.Type()

	// Check if this node is a symbol definition
	if kind, ok := cfg.SymbolKinds[nodeType]; ok {
		sym := extractSymbolFromNode(node, kind, file, lang, content, scope)
		if sym != nil {
			*symbols = append(*symbols, *sym)
			// Update scope for children
			if kind == "class" || kind == "type" || kind == "function" || kind == "method" {
				scope = sym.Name
			}
		}
	}

	// Check if this node is a reference
	if kind, ok := cfg.RefKinds[nodeType]; ok {
		ref := extractRefFromNode(node, kind, file, content, scope)
		if ref != nil {
			*refs = append(*refs, *ref)
		}
	}

	// Recurse into children
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		walkNode(child, cfg, file, lang, content, scope, symbols, refs)
	}
}

// extractSymbolFromNode extracts a Symbol from an AST node.
func extractSymbolFromNode(node *sitter.Node, kind, file, lang string, content []byte, scope string) *Symbol {
	// Find the name node - varies by language and node type
	var name string
	var signature string

	// Try common field names for the identifier
	for _, fieldName := range []string{"name", "declarator", "left"} {
		if nameNode := node.ChildByFieldName(fieldName); nameNode != nil {
			// Handle nested declarators (e.g., variable_declarator has name inside)
			if nameNode.Type() == "identifier" || nameNode.Type() == "type_identifier" {
				name = nameNode.Content(content)
				break
			}
			// For declarators, look for identifier inside
			for i := 0; i < int(nameNode.ChildCount()); i++ {
				child := nameNode.Child(i)
				if child.Type() == "identifier" {
					name = child.Content(content)
					break
				}
			}
			if name != "" {
				break
			}
		}
	}

	if name == "" {
		// Fallback: find first identifier child (including field/property identifiers for methods)
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			ct := child.Type()
			if ct == "identifier" || ct == "type_identifier" || ct == "field_identifier" || ct == "property_identifier" {
				name = child.Content(content)
				break
			}
		}
	}

	if name == "" {
		return nil
	}

	// Extract signature for functions/methods
	if kind == "function" || kind == "method" {
		if params := node.ChildByFieldName("parameters"); params != nil {
			signature = "func " + name + params.Content(content)
			if result := node.ChildByFieldName("result"); result != nil {
				signature += " " + result.Content(content)
			}
		}
	}

	return &Symbol{
		Name:      name,
		Kind:      kind,
		File:      file,
		Line:      int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		Column:    int(node.StartPoint().Column) + 1,
		StartByte: node.StartByte(),
		EndByte:   node.EndByte(),
		Language:  lang,
		Scope:     scope,
		Signature: signature,
	}
}

// extractRefFromNode extracts a Reference from an AST node.
func extractRefFromNode(node *sitter.Node, kind, file string, content []byte, caller string) *Reference {
	// For call expressions, find the function being called
	var symbol string

	if funcNode := node.ChildByFieldName("function"); funcNode != nil {
		symbol = funcNode.Content(content)
	} else if node.ChildCount() > 0 {
		// First child is often the function/method being called
		symbol = node.Child(0).Content(content)
	}

	if symbol == "" {
		return nil
	}

	return &Reference{
		Symbol: symbol,
		File:   file,
		Line:   int(node.StartPoint().Row) + 1,
		Column: int(node.StartPoint().Column) + 1,
		Caller: caller,
		Kind:   kind,
	}
}
