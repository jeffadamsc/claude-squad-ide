// app/indexer_persistence.go
package app

import (
	"encoding/gob"
	"encoding/json"
	"os"
	"path/filepath"
)

type IndexStore struct {
	baseDir string
}

type indexMeta struct {
	Version     int    `json:"version"`
	Commit      string `json:"commit"`
	SymbolCount int    `json:"symbol_count"`
	RefCount    int    `json:"ref_count"`
}

func NewIndexStore(worktree string) *IndexStore {
	return &IndexStore{baseDir: worktree}
}

func (s *IndexStore) indexDir() string {
	return filepath.Join(s.baseDir, ".claude-squad", "index")
}

func (s *IndexStore) Save(symbols map[string][]Symbol, cg *CallGraph, commit string) error {
	dir := s.indexDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	symbolCount := 0
	for _, syms := range symbols {
		symbolCount += len(syms)
	}

	callerCount, _ := cg.Stats()

	meta := indexMeta{
		Version:     1,
		Commit:      commit,
		SymbolCount: symbolCount,
		RefCount:    callerCount,
	}
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), metaData, 0644); err != nil {
		return err
	}

	symFile, err := os.Create(filepath.Join(dir, "symbols.gob"))
	if err != nil {
		return err
	}
	defer symFile.Close()
	if err := gob.NewEncoder(symFile).Encode(symbols); err != nil {
		return err
	}

	cg.mu.RLock()
	cgData := struct {
		Callers map[string][]Reference
		Callees map[string][]Reference
	}{
		Callers: cg.callers,
		Callees: cg.callees,
	}
	cg.mu.RUnlock()

	cgFile, err := os.Create(filepath.Join(dir, "callgraph.gob"))
	if err != nil {
		return err
	}
	defer cgFile.Close()
	if err := gob.NewEncoder(cgFile).Encode(cgData); err != nil {
		return err
	}

	return nil
}

func (s *IndexStore) Load() (map[string][]Symbol, *CallGraph, string, error) {
	dir := s.indexDir()

	metaPath := filepath.Join(dir, "meta.json")
	metaData, err := os.ReadFile(metaPath)
	if os.IsNotExist(err) {
		return nil, nil, "", nil
	}
	if err != nil {
		return nil, nil, "", err
	}

	var meta indexMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return nil, nil, "", err
	}

	if meta.Version != 1 {
		return nil, nil, "", nil
	}

	symFile, err := os.Open(filepath.Join(dir, "symbols.gob"))
	if err != nil {
		return nil, nil, "", err
	}
	defer symFile.Close()

	var symbols map[string][]Symbol
	if err := gob.NewDecoder(symFile).Decode(&symbols); err != nil {
		return nil, nil, "", err
	}

	cgFile, err := os.Open(filepath.Join(dir, "callgraph.gob"))
	if err != nil {
		return nil, nil, "", err
	}
	defer cgFile.Close()

	var cgData struct {
		Callers map[string][]Reference
		Callees map[string][]Reference
	}
	if err := gob.NewDecoder(cgFile).Decode(&cgData); err != nil {
		return nil, nil, "", err
	}

	cg := &CallGraph{
		callers: cgData.Callers,
		callees: cgData.Callees,
	}

	return symbols, cg, meta.Commit, nil
}

func (s *IndexStore) GetCommit() string {
	metaPath := filepath.Join(s.indexDir(), "meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return ""
	}
	var meta indexMeta
	if json.Unmarshal(data, &meta) != nil {
		return ""
	}
	return meta.Commit
}
