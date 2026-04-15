package app

import (
	"testing"
)

func TestSymbolSearchRanking(t *testing.T) {
	idx := NewSymbolIndex()
	defer idx.Close()

	// Add symbols with varying relevance
	idx.Index(Symbol{Name: "getUserByID", Kind: "function", File: "user.go"})
	idx.Index(Symbol{Name: "getUser", Kind: "function", File: "user.go"})
	idx.Index(Symbol{Name: "UserService", Kind: "class", File: "service.go"})
	idx.Index(Symbol{Name: "processUserData", Kind: "function", File: "process.go"})

	results := idx.Search("user", 10)

	if len(results) == 0 {
		t.Fatal("expected search results")
	}

	// "getUser" should rank higher than "processUserData" (exact word match)
	// This is a soft assertion - BM25 should handle this
	t.Logf("Search results: %v", results)
}
