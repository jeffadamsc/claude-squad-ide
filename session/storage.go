package session

import (
	"claude-squad/config"
	"encoding/json"
	"fmt"
	"time"
)

// InstanceData represents the serializable data of an Instance
type InstanceData struct {
	Title     string    `json:"title"`
	Path      string    `json:"path"`
	Branch    string    `json:"branch"`
	Status    Status    `json:"status"`
	Height    int       `json:"height"`
	Width     int       `json:"width"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	AutoYes   bool      `json:"auto_yes"`
	InPlace   bool      `json:"in_place,omitempty"`

	Program          string          `json:"program"`
	ClaudeSessionID  string          `json:"claude_session_id,omitempty"`
	Worktree  GitWorktreeData `json:"worktree"`
	DiffStats DiffStatsData   `json:"diff_stats"`
}

// GitWorktreeData represents the serializable data of a GitWorktree
type GitWorktreeData struct {
	RepoPath         string `json:"repo_path"`
	WorktreePath     string `json:"worktree_path"`
	SessionName      string `json:"session_name"`
	BranchName       string `json:"branch_name"`
	BaseCommitSHA    string `json:"base_commit_sha"`
	IsExistingBranch bool   `json:"is_existing_branch"`
}

// DiffStatsData represents the serializable data of a DiffStats
type DiffStatsData struct {
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Content string `json:"content"`
}

// Storage handles saving and loading instances using the state interface
type Storage struct {
	state config.InstanceStorage
}

// NewStorage creates a new storage instance
func NewStorage(state config.InstanceStorage) (*Storage, error) {
	return &Storage{
		state: state,
	}, nil
}

// SaveInstances saves the list of instances to disk
func (s *Storage) SaveInstances(instances []*Instance) error {
	// Convert instances to InstanceData
	data := make([]InstanceData, 0)
	for _, instance := range instances {
		if instance.Started() {
			data = append(data, instance.ToInstanceData())
		}
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}

	return s.state.SaveInstances(jsonData)
}

// LoadInstances loads the list of instances from disk.
// The ProcessManager is passed to FromInstanceData for restoring non-paused sessions.
func (s *Storage) LoadInstances(pm ProcessManager) ([]*Instance, error) {
	jsonData := s.state.GetInstances()

	var instancesData []InstanceData
	if err := json.Unmarshal(jsonData, &instancesData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal instances: %w", err)
	}

	instances := make([]*Instance, len(instancesData))
	for i, data := range instancesData {
		instance, err := FromInstanceData(data, pm)
		if err != nil {
			return nil, fmt.Errorf("failed to create instance %s: %w", data.Title, err)
		}
		instances[i] = instance
	}

	return instances, nil
}

// LoadInstancesData loads the raw instance data without restoring processes.
func (s *Storage) LoadInstancesData() ([]InstanceData, error) {
	jsonData := s.state.GetInstances()

	var instancesData []InstanceData
	if err := json.Unmarshal(jsonData, &instancesData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal instances: %w", err)
	}
	return instancesData, nil
}

// SaveInstancesData saves raw instance data to disk.
func (s *Storage) SaveInstancesData(data []InstanceData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}
	return s.state.SaveInstances(jsonData)
}

// DeleteInstance removes an instance from storage
func (s *Storage) DeleteInstance(title string) error {
	allData, err := s.LoadInstancesData()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	found := false
	filtered := make([]InstanceData, 0, len(allData))
	for _, d := range allData {
		if d.Title != title {
			filtered = append(filtered, d)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", title)
	}

	return s.SaveInstancesData(filtered)
}

// UpdateInstance updates an existing instance in storage
func (s *Storage) UpdateInstance(instance *Instance) error {
	allData, err := s.LoadInstancesData()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	data := instance.ToInstanceData()
	found := false
	for i, existing := range allData {
		if existing.Title == data.Title {
			allData[i] = data
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", data.Title)
	}

	return s.SaveInstancesData(allData)
}

// DeleteAllInstances removes all stored instances
func (s *Storage) DeleteAllInstances() error {
	return s.state.DeleteAllInstances()
}
