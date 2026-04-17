package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
)

// SymbolIndex provides BM25-ranked search over symbols.
type SymbolIndex struct {
	index bleve.Index
	path  string // on-disk path (empty = in-memory)
}

// NewSymbolIndexOnDisk creates a disk-backed bleve index at the given path.
// Falls back to in-memory if disk creation fails.
func NewSymbolIndexOnDisk(dir string) *SymbolIndex {
	path := filepath.Join(dir, ".claude-squad", "index", "bleve.index")
	// Remove stale index from a previous run
	os.RemoveAll(path)
	m := buildIndexMapping()
	index, err := bleve.New(path, m)
	if err != nil {
		// Fallback to in-memory
		index, _ = bleve.NewMemOnly(m)
		return &SymbolIndex{index: index}
	}
	return &SymbolIndex{index: index, path: path}
}

func buildIndexMapping() mapping.IndexMapping {
	textFieldMapping := bleve.NewTextFieldMapping()
	textFieldMapping.Analyzer = "standard"

	keywordFieldMapping := bleve.NewKeywordFieldMapping()

	numericFieldMapping := bleve.NewNumericFieldMapping()

	symbolMapping := bleve.NewDocumentMapping()
	symbolMapping.AddFieldMappingsAt("name", textFieldMapping)
	symbolMapping.AddFieldMappingsAt("kind", keywordFieldMapping)
	symbolMapping.AddFieldMappingsAt("file", keywordFieldMapping)
	symbolMapping.AddFieldMappingsAt("language", keywordFieldMapping)
	symbolMapping.AddFieldMappingsAt("scope", textFieldMapping)
	symbolMapping.AddFieldMappingsAt("signature", textFieldMapping)
	symbolMapping.AddFieldMappingsAt("line", numericFieldMapping)
	symbolMapping.AddFieldMappingsAt("end_line", numericFieldMapping)
	symbolMapping.AddFieldMappingsAt("column", numericFieldMapping)
	symbolMapping.AddFieldMappingsAt("start_byte", numericFieldMapping)
	symbolMapping.AddFieldMappingsAt("end_byte", numericFieldMapping)

	indexMapping := bleve.NewIndexMapping()
	indexMapping.DefaultMapping = symbolMapping

	return indexMapping
}

type symbolDoc struct {
	Symbol
	ID string `json:"id"`
}

func (si *SymbolIndex) Index(sym Symbol) error {
	doc := symbolDoc{
		Symbol: sym,
		ID:     sym.File + ":" + sym.Name,
	}
	return si.index.Index(doc.ID, doc)
}

func (si *SymbolIndex) IndexBatch(symbols []Symbol) error {
	batch := si.index.NewBatch()
	for _, sym := range symbols {
		doc := symbolDoc{
			Symbol: sym,
			ID:     sym.File + ":" + sym.Name,
		}
		batch.Index(doc.ID, doc)
	}
	return si.index.Batch(batch)
}

func (si *SymbolIndex) Search(query string, limit int) []Symbol {
	// Try an exact query string first; if it returns no hits, fall back to a
	// wildcard query so camelCase symbol names like "getUserByID" are matched
	// when searching for a substring like "user".
	q := bleve.NewQueryStringQuery(query)
	req := bleve.NewSearchRequest(q)
	req.Size = limit
	req.Fields = []string{"name", "kind", "file", "line", "end_line",
		"column", "language", "scope", "signature", "start_byte", "end_byte"}

	results, err := si.index.Search(req)
	if err != nil {
		return nil
	}

	if results.Total == 0 {
		// Fallback: wildcard match on the name field (case-insensitive via lowercase)
		// The standard analyzer lowercases indexed terms, so we must lowercase the query too
		wq := bleve.NewWildcardQuery("*" + strings.ToLower(query) + "*")
		wq.SetField("name")
		req2 := bleve.NewSearchRequest(wq)
		req2.Size = limit
		req2.Fields = req.Fields
		results, err = si.index.Search(req2)
		if err != nil {
			return nil
		}
	}

	var symbols []Symbol
	for _, hit := range results.Hits {
		sym := Symbol{
			Name:      getString(hit.Fields, "name"),
			Kind:      getString(hit.Fields, "kind"),
			File:      getString(hit.Fields, "file"),
			Line:      getInt(hit.Fields, "line"),
			EndLine:   getInt(hit.Fields, "end_line"),
			Column:    getInt(hit.Fields, "column"),
			StartByte: getUint32(hit.Fields, "start_byte"),
			EndByte:   getUint32(hit.Fields, "end_byte"),
			Language:  getString(hit.Fields, "language"),
			Scope:     getString(hit.Fields, "scope"),
			Signature: getString(hit.Fields, "signature"),
		}
		symbols = append(symbols, sym)
	}

	return symbols
}

func (si *SymbolIndex) Clear() error {
	si.index.Close()
	m := buildIndexMapping()
	if si.path != "" {
		os.RemoveAll(si.path)
		index, err := bleve.New(si.path, m)
		if err != nil {
			// Fallback to in-memory
			index, _ = bleve.NewMemOnly(m)
			si.path = ""
		}
		si.index = index
	} else {
		index, err := bleve.NewMemOnly(m)
		if err != nil {
			return err
		}
		si.index = index
	}
	return nil
}

func (si *SymbolIndex) Close() error {
	err := si.index.Close()
	if si.path != "" {
		os.RemoveAll(si.path)
	}
	return err
}

func getString(fields map[string]interface{}, key string) string {
	if v, ok := fields[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt(fields map[string]interface{}, key string) int {
	if v, ok := fields[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return 0
}

func getUint32(fields map[string]interface{}, key string) uint32 {
	if v, ok := fields[key]; ok {
		switch n := v.(type) {
		case float64:
			return uint32(n)
		case int:
			return uint32(n)
		case uint32:
			return n
		}
	}
	return 0
}
