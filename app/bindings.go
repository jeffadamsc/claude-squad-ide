package app

import (
	"fmt"
	"sync"

	"claude-squad/config"
	"claude-squad/log"
	ptyPkg "claude-squad/pty"
	"claude-squad/session"
)

type SessionAPIOptions struct {
	DataDir string
}

type CreateOptions struct {
	Title   string `json:"title"`
	Path    string `json:"path"`
	Program string `json:"program"`
	Branch  string `json:"branch"`
	AutoYes bool   `json:"autoYes"`
	InPlace bool   `json:"inPlace"`
}

type SessionInfo struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Path    string `json:"path"`
	Branch  string `json:"branch"`
	Program string `json:"program"`
	Status  string `json:"status"`
}

type SessionStatus struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Branch    string    `json:"branch"`
	DiffStats DiffStats `json:"diffStats"`
	HasPrompt bool      `json:"hasPrompt"`
}

type DiffStats struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
}

type SessionAPI struct {
	mu         sync.RWMutex
	instances  map[string]*session.Instance
	storage    *session.Storage
	ptyManager *ptyPkg.Manager
	wsServer   *ptyPkg.WebSocketServer
	wsPort     int
	cfg        *config.Config
}

func NewSessionAPI(opts SessionAPIOptions) (*SessionAPI, error) {
	mgr := ptyPkg.NewManager()
	ws := ptyPkg.NewWebSocketServer(mgr)

	port, err := ws.ListenAndServe()
	if err != nil {
		return nil, fmt.Errorf("start websocket server: %w", err)
	}

	cfg := config.LoadConfig()
	state := config.LoadState()
	storage, err := session.NewStorage(state)
	if err != nil {
		return nil, fmt.Errorf("init storage: %w", err)
	}

	api := &SessionAPI{
		instances:  make(map[string]*session.Instance),
		storage:    storage,
		ptyManager: mgr,
		wsServer:   ws,
		wsPort:     port,
		cfg:        cfg,
	}

	// Load persisted sessions as metadata. Sessions that were running when
	// last saved are loaded as paused — the process is gone, so the user
	// must explicitly resume them.
	allData, err := storage.LoadInstancesData()
	if err != nil {
		log.ErrorLog.Printf("failed to load persisted sessions: %v", err)
	} else {
		for _, data := range allData {
			// Force all loaded sessions to paused state so we don't try to
			// spawn processes for sessions that were interrupted.
			if data.Status != session.Paused {
				data.Status = session.Paused
			}
			inst, err := session.FromInstanceData(data, mgr)
			if err != nil {
				log.ErrorLog.Printf("failed to restore session %s: %v", data.Title, err)
				continue
			}
			api.instances[inst.Title] = inst
		}
	}

	return api, nil
}

func statusString(s session.Status) string {
	switch s {
	case session.Running:
		return "running"
	case session.Ready:
		return "ready"
	case session.Loading:
		return "loading"
	case session.Paused:
		return "paused"
	default:
		return "unknown"
	}
}

func instanceToInfo(inst *session.Instance) SessionInfo {
	return SessionInfo{
		ID:      inst.Title,
		Title:   inst.Title,
		Path:    inst.Path,
		Branch:  inst.Branch,
		Program: inst.Program,
		Status:  statusString(inst.Status),
	}
}

func (api *SessionAPI) CreateSession(opts CreateOptions) (*SessionInfo, error) {
	api.mu.Lock()
	defer api.mu.Unlock()

	program := opts.Program
	if program == "" {
		program = api.cfg.DefaultProgram
	}

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:          opts.Title,
		Path:           opts.Path,
		Program:        program,
		AutoYes:        opts.AutoYes,
		Branch:         opts.Branch,
		InPlace:        opts.InPlace,
		ProcessManager: api.ptyManager,
	})
	if err != nil {
		return nil, fmt.Errorf("create instance: %w", err)
	}

	api.instances[inst.Title] = inst
	api.saveInstancesLocked()

	info := instanceToInfo(inst)
	return &info, nil
}

func (api *SessionAPI) StartSession(id string) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	inst, ok := api.instances[id]
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}

	if inst.Started() {
		return nil
	}

	if err := inst.Start(true); err != nil {
		return fmt.Errorf("start session: %w", err)
	}

	api.saveInstancesLocked()
	return nil
}

func (api *SessionAPI) PauseSession(id string) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	inst, ok := api.instances[id]
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}

	if err := inst.Pause(); err != nil {
		return fmt.Errorf("pause session: %w", err)
	}

	api.saveInstancesLocked()
	return nil
}

func (api *SessionAPI) ResumeSession(id string) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	inst, ok := api.instances[id]
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}

	if err := inst.Resume(); err != nil {
		return fmt.Errorf("resume session: %w", err)
	}

	api.saveInstancesLocked()
	return nil
}

func (api *SessionAPI) KillSession(id string) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	inst, ok := api.instances[id]
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}

	if err := inst.Kill(); err != nil {
		return fmt.Errorf("kill session: %w", err)
	}

	delete(api.instances, id)
	api.saveInstancesLocked()
	return nil
}

func (api *SessionAPI) DeleteSession(id string) error {
	return api.KillSession(id)
}

func (api *SessionAPI) LoadSessions() ([]SessionInfo, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	result := make([]SessionInfo, 0, len(api.instances))
	for _, inst := range api.instances {
		result = append(result, instanceToInfo(inst))
	}
	return result, nil
}

func (api *SessionAPI) GetSessionStatus(id string) (*SessionStatus, error) {
	api.mu.RLock()
	inst, ok := api.instances[id]
	api.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session %s not found", id)
	}

	updated, hasPrompt := inst.HasUpdated()
	_ = updated

	var ds DiffStats
	if stats := inst.GetDiffStats(); stats != nil {
		ds = DiffStats{Added: stats.Added, Removed: stats.Removed}
	}

	return &SessionStatus{
		ID:        inst.Title,
		Status:    statusString(inst.Status),
		Branch:    inst.Branch,
		DiffStats: ds,
		HasPrompt: hasPrompt,
	}, nil
}

func (api *SessionAPI) PollAllStatuses() ([]SessionStatus, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	result := make([]SessionStatus, 0, len(api.instances))
	for _, inst := range api.instances {
		_, hasPrompt := inst.HasUpdated()

		var ds DiffStats
		if stats := inst.GetDiffStats(); stats != nil {
			ds = DiffStats{Added: stats.Added, Removed: stats.Removed}
		}

		result = append(result, SessionStatus{
			ID:        inst.Title,
			Status:    statusString(inst.Status),
			Branch:    inst.Branch,
			DiffStats: ds,
			HasPrompt: hasPrompt,
		})
	}
	return result, nil
}

func (api *SessionAPI) SendPrompt(id string, prompt string) error {
	api.mu.RLock()
	inst, ok := api.instances[id]
	api.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	return inst.SendPrompt(prompt)
}

func (api *SessionAPI) GetWebSocketPort() int {
	return api.wsPort
}

func (api *SessionAPI) GetConfig() (*config.Config, error) {
	return api.cfg, nil
}

func (api *SessionAPI) Close() {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Save state before closing
	api.saveInstancesLocked()
	api.ptyManager.Close()
}

// saveInstancesLocked persists all instances to disk. Must be called with api.mu held.
func (api *SessionAPI) saveInstancesLocked() {
	instances := make([]*session.Instance, 0, len(api.instances))
	for _, inst := range api.instances {
		instances = append(instances, inst)
	}
	if err := api.storage.SaveInstances(instances); err != nil {
		log.ErrorLog.Printf("failed to save instances: %v", err)
	}
}
