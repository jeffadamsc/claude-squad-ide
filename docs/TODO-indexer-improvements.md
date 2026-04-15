# Tree-Sitter Indexer Improvement TODO

Based on analysis of pitlane-mcp, here are potential improvements to get closer to 95% token savings.

## Current State
- 41% token reduction with `smart_lookup` context bundling
- BM25 search via bleve
- Call graph analysis
- Incremental indexing with mtime tracking

## High-Impact Improvements

### 1. Guidance Blocks in Responses
**Impact: High** - Steers Claude away from wasteful grep/read patterns

Add explicit guidance to every search response:
- `next_step`: "Call get_symbol_source on top results before more searches"
- `avoid`: "Don't run parallel broad searches"
- `query_hint`: Warn when query is too generic (single common words)

Example:
```json
{
  "results": [...],
  "guidance": {
    "next_step": "Call smart_lookup on the top 1-2 results",
    "avoid": "Avoid launching more discovery searches in parallel"
  }
}
```

### 2. Signature-Only Mode for Container Types
**Impact: High** - Avoids dumping massive class/struct bodies

For structs, classes, interfaces:
- Return only signature + docstring by default
- Full body only with `include_body: true`
- Saves tokens when agent just needs the interface

### 3. Broad Query Detection
**Impact: Medium** - Prevents wasteful searches

Detect overly generic queries:
- Single common words: "search", "handle", "process", "get", "set"
- Warn user to be more specific
- Suggest adding type/file filters

### 4. Answer-Now Hints
**Impact: Medium** - Tells agent when it has enough context

After `smart_lookup` returns bundled context:
- Add hint: "You likely have enough context to answer - avoid additional lookups"
- Track what's been retrieved to avoid redundant calls

### 5. Custom Code Tokenizer for BM25
**Impact: Medium** - Better search relevance

Split identifiers properly:
- `parseHTTPResponse` → `["parse", "http", "response"]`
- `snake_case_name` → `["snake", "case", "name"]`
- Improves search recall for partial matches

### 6. References Field in Symbol Responses
**Impact: Medium** - Pre-compute call dependencies

Include in `get_symbol_source` response:
- `references`: list of symbols called within the source
- Saves separate `find_references` call
- Already have call graph data, just need to expose it

### 7. Token Usage Stats Tool
**Impact: Low** - Visibility into savings

Add `get_usage_stats` tool:
- Track `full_source_bytes` vs `returned_bytes`
- Show `tokens_saved_approx`
- Helps demonstrate value and tune behavior

## Medium-Impact Improvements

### 8. trace_execution_path Tool
Single-call path tracing:
- Input: entry point symbol
- Output: categorized symbols (entry, orchestration, core, output layers)
- Includes call edges and snippets
- Replaces multiple get_symbol calls

### 9. Project Outline Summary Mode
For large codebases:
- `summary: true` returns only directory structure with counts
- Collapse directories with >200 items
- Shows file/symbol counts per directory

### 10. Noise Filtering
Deprioritize in search results:
- Test files (`*_test.go`, `test_*.py`)
- Example/demo code
- Generated code
- Vendor/node_modules (already excluded from index)

## Lower Priority

### 11. Semantic Search (Embeddings)
- Local embeddings via Ollama
- Cosine similarity for intent-based queries
- Incremental embedding updates
- Higher implementation cost

### 12. Disk-Persisted Index
- Already have IndexStore, could optimize serialization
- Use bincode/msgpack for faster load
- Track file mtimes in metadata

## Implementation Order (Recommended)

1. **Guidance blocks** - Quick win, big impact on behavior
2. **Signature-only mode** - Significant token savings for types
3. **References in responses** - Already have data, easy to add
4. **Broad query detection** - Simple heuristics
5. **Answer-now hints** - Builds on guidance blocks
6. **Custom tokenizer** - Moderate effort, good search improvement
7. **Trace execution path** - Bigger feature, high value

## Metrics to Track

- Input tokens per task (primary metric)
- MCP tool call count vs grep/read count
- Time to answer
- Answer quality (manual review)
