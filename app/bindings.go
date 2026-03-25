package app

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"claude-squad/config"
	"claude-squad/log"
	ptyPkg "claude-squad/pty"
	"claude-squad/session"
	"claude-squad/session/git"
	sshPkg "claude-squad/ssh"
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
	Prompt  string `json:"prompt"`
	HostID  string `json:"hostId"`
}

type SessionInfo struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Path    string `json:"path"`
	Branch  string `json:"branch"`
	Program string `json:"program"`
	Status  string `json:"status"`
	HostID  string `json:"hostId"`
}

type SessionStatus struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	Branch       string    `json:"branch"`
	DiffStats    DiffStats `json:"diffStats"`
	HasPrompt    bool      `json:"hasPrompt"`
	SSHConnected *bool     `json:"sshConnected"`
}

type DiffStats struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
}

type SessionAPI struct {
	mu            sync.RWMutex
	instances     map[string]*session.Instance
	storage       *session.Storage
	ptyManager    *ptyPkg.Manager
	wsServer      *ptyPkg.WebSocketServer
	wsPort        int
	cfg           *config.Config
	dirty         bool // true when instances have been modified and need saving
	hostManager   *sshPkg.HostManager
	hostStore     *sshPkg.HostStore
	keychainStore *sshPkg.KeychainStore
}

func NewSessionAPI(opts SessionAPIOptions) (*SessionAPI, error) {
	mgr := ptyPkg.NewManager()
	ws := ptyPkg.NewWebSocketServer(mgr, mgr)

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

	configDir, _ := config.GetConfigDir()
	hostStore := sshPkg.NewHostStore(filepath.Join(configDir, "hosts.json"))
	keychainStore := sshPkg.NewKeychainStore("com.claude-squad")
	hostMgr := sshPkg.NewHostManager(hostStore, keychainStore)

	api := &SessionAPI{
		instances:     make(map[string]*session.Instance),
		storage:       storage,
		ptyManager:    mgr,
		wsServer:      ws,
		wsPort:        port,
		cfg:           cfg,
		hostManager:   hostMgr,
		hostStore:     hostStore,
		keychainStore: keychainStore,
	}

	// Load persisted sessions as metadata. Sessions that were running when
	// last saved are loaded as paused — the process is gone, so the user
	// must explicitly resume them.
	allData, err := storage.LoadInstancesData()
	if err != nil {
		log.ErrorLog.Printf("failed to load persisted sessions: %v", err)
	} else {
		log.InfoLog.Printf("loaded %d persisted sessions from state.json", len(allData))
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
			log.InfoLog.Printf("restored session: %s (branch: %s)", inst.Title, inst.Branch)
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
		HostID:  inst.HostID,
	}
}

func (api *SessionAPI) CreateSession(opts CreateOptions) (*SessionInfo, error) {
	api.mu.Lock()
	defer api.mu.Unlock()

	program := opts.Program
	if program == "" {
		program = api.cfg.DefaultProgram
	}

	var pm session.ProcessManager = api.ptyManager
	if opts.HostID != "" {
		if _, err := api.hostManager.GetClient(opts.HostID); err != nil {
			return nil, fmt.Errorf("connect to remote host: %w", err)
		}
		sshPM, err := api.hostManager.GetProcessManager(opts.HostID)
		if err != nil {
			return nil, fmt.Errorf("get ssh process manager: %w", err)
		}
		pm = sshPM
	}

	inst, err := session.NewInstance(session.InstanceOptions{
		Title:          opts.Title,
		Path:           opts.Path,
		Program:        program,
		AutoYes:        opts.AutoYes,
		Branch:         opts.Branch,
		InPlace:        opts.InPlace,
		Prompt:         opts.Prompt,
		HostID:         opts.HostID,
		ProcessManager: pm,
	})
	if err != nil {
		return nil, fmt.Errorf("create instance: %w", err)
	}

	api.instances[inst.Title] = inst
	api.dirty = true
	api.saveInstancesLocked()

	info := instanceToInfo(inst)
	return &info, nil
}

// OpenSession ensures the session has a running process and returns the PTY
// session ID for WebSocket connection. Resumes paused sessions automatically.
func (api *SessionAPI) OpenSession(id string) (string, error) {
	log.InfoLog.Printf("OpenSession called for id=%q", id)

	api.mu.Lock()
	defer api.mu.Unlock()

	inst, ok := api.instances[id]
	if !ok {
		log.ErrorLog.Printf("OpenSession: session %q not found (have %d instances)", id, len(api.instances))
		return "", fmt.Errorf("session %s not found", id)
	}

	// Reconnect remote sessions if needed
	if inst.HostID != "" {
		if _, err := api.hostManager.GetClient(inst.HostID); err != nil {
			return "", fmt.Errorf("reconnect to remote host: %w", err)
		}
		pm, err := api.hostManager.GetProcessManager(inst.HostID)
		if err != nil {
			return "", fmt.Errorf("get ssh process manager: %w", err)
		}
		inst.SetProcessManager(pm)
	}

	// Resume paused sessions
	if inst.Paused() {
		log.InfoLog.Printf("OpenSession: resuming paused session %q", id)
		if err := inst.Resume(); err != nil {
			log.ErrorLog.Printf("OpenSession: resume failed for %q: %v", id, err)
			return "", fmt.Errorf("resume session: %w", err)
		}
		api.dirty = true
		api.saveInstancesLocked()
	}

	// Return the PTY process ID for WebSocket routing
	ptyID := inst.GetProcessID()
	if ptyID == "" {
		log.ErrorLog.Printf("OpenSession: session %q has no running process", id)
		return "", fmt.Errorf("session %s has no running process", id)
	}

	log.InfoLog.Printf("OpenSession: returning ptyID=%q for session %q", ptyID, id)
	return ptyID, nil
}

func (api *SessionAPI) StartSession(id string) error {
	api.mu.Lock()
	inst, ok := api.instances[id]
	if !ok {
		api.mu.Unlock()
		return fmt.Errorf("session %s not found", id)
	}
	if inst.Started() {
		api.mu.Unlock()
		return nil
	}
	// Set loading status while holding the lock so the poller can see it
	inst.SetStatus(session.Loading)
	api.mu.Unlock()

	// Start is slow (git worktree setup, process spawn) — run without holding the lock
	// so the status poller can continue to report "loading" state
	if err := inst.Start(true); err != nil {
		return fmt.Errorf("start session: %w", err)
	}

	api.mu.Lock()
	api.dirty = true
	api.saveInstancesLocked()
	api.mu.Unlock()
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

	api.dirty = true
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

	api.dirty = true
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
	api.dirty = true
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

		var sshConnected *bool
		if inst.HostID != "" {
			connected := api.hostManager.IsConnected(inst.HostID)
			sshConnected = &connected
		}

		result = append(result, SessionStatus{
			ID:           inst.Title,
			Status:       statusString(inst.Status),
			Branch:       inst.Branch,
			DiffStats:    ds,
			HasPrompt:    hasPrompt,
			SSHConnected: sshConnected,
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

type DirInfo struct {
	DefaultBranch string   `json:"defaultBranch"`
	Branches      []string `json:"branches"`
}

// GetDirInfo returns the default branch and branch list for a directory.
func (api *SessionAPI) GetDirInfo(dir string) (*DirInfo, error) {
	defaultBranch := git.GetDefaultBranch(dir)
	branches, err := git.SearchBranches(dir, "")
	if err != nil {
		branches = []string{}
	}
	return &DirInfo{
		DefaultBranch: defaultBranch,
		Branches:      branches,
	}, nil
}

// SearchBranches searches for branches matching a filter in a directory.
func (api *SessionAPI) SearchBranches(dir string, filter string) ([]string, error) {
	branches, err := git.SearchBranches(dir, filter)
	if err != nil {
		return []string{}, nil
	}
	return branches, nil
}

type AppConfig struct {
	DefaultProgram string           `json:"DefaultProgram"`
	AutoYes        bool             `json:"AutoYes"`
	BranchPrefix   string           `json:"BranchPrefix"`
	Profiles       []config.Profile `json:"Profiles"`
	DefaultWorkDir string           `json:"DefaultWorkDir"`
}

func (api *SessionAPI) GetConfig() (*AppConfig, error) {
	return &AppConfig{
		DefaultProgram: api.cfg.DefaultProgram,
		AutoYes:        api.cfg.AutoYes,
		BranchPrefix:   api.cfg.BranchPrefix,
		Profiles:       api.cfg.GetProfiles(),
		DefaultWorkDir: api.cfg.DefaultWorkDir,
	}, nil
}

func (api *SessionAPI) Close() {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Only save state if we actually modified something
	api.saveInstancesLocked()
	api.ptyManager.Close()
	if api.hostManager != nil {
		api.hostManager.Close()
	}
}

// --- Host API Methods ---

type HostInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	AuthMethod string `json:"authMethod"`
	KeyPath    string `json:"keyPath"`
}

type CreateHostOptions struct {
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	AuthMethod string `json:"authMethod"`
	KeyPath    string `json:"keyPath"`
	Secret     string `json:"secret"` // password or passphrase — stored in keychain, not persisted
}

type TestHostResult struct {
	ConnectionOK bool   `json:"connectionOK"`
	ProgramOK    bool   `json:"programOK"`
	Message      string `json:"message"`
}

func (api *SessionAPI) GetHosts() ([]HostInfo, error) {
	hosts, err := api.hostStore.LoadAll()
	if err != nil {
		return nil, err
	}
	result := make([]HostInfo, len(hosts))
	for i, h := range hosts {
		result[i] = HostInfo{
			ID: h.ID, Name: h.Name, Host: h.Host,
			Port: h.Port, User: h.User, AuthMethod: h.AuthMethod,
			KeyPath: h.KeyPath,
		}
	}
	return result, nil
}

func (api *SessionAPI) CreateHost(opts CreateHostOptions) (*HostInfo, error) {
	id := generateAppUUID()
	config := sshPkg.HostConfig{
		ID: id, Name: opts.Name, Host: opts.Host,
		Port: opts.Port, User: opts.User, AuthMethod: opts.AuthMethod,
		KeyPath: opts.KeyPath,
	}
	if err := api.hostStore.Save(config); err != nil {
		return nil, err
	}
	if opts.Secret != "" {
		if err := api.keychainStore.Set(id, opts.Secret); err != nil {
			_ = api.hostStore.Delete(id)
			return nil, fmt.Errorf("store secret: %w", err)
		}
	}
	info := HostInfo{
		ID: id, Name: opts.Name, Host: opts.Host,
		Port: opts.Port, User: opts.User, AuthMethod: opts.AuthMethod,
		KeyPath: opts.KeyPath,
	}
	return &info, nil
}

func (api *SessionAPI) UpdateHost(opts CreateHostOptions, id string) error {
	config := sshPkg.HostConfig{
		ID: id, Name: opts.Name, Host: opts.Host,
		Port: opts.Port, User: opts.User, AuthMethod: opts.AuthMethod,
		KeyPath: opts.KeyPath,
	}
	if err := api.hostStore.Update(config); err != nil {
		return err
	}
	if opts.Secret != "" {
		if err := api.keychainStore.Set(id, opts.Secret); err != nil {
			return fmt.Errorf("update secret: %w", err)
		}
	}
	return nil
}

func (api *SessionAPI) DeleteHost(id string) error {
	api.mu.RLock()
	for _, inst := range api.instances {
		if inst.HostID == id && inst.Status != session.Paused {
			api.mu.RUnlock()
			return fmt.Errorf("host has active sessions — pause them first")
		}
	}
	api.mu.RUnlock()

	_ = api.keychainStore.Delete(id)
	return api.hostStore.Delete(id)
}

func (api *SessionAPI) TestHost(opts CreateHostOptions, program string) (*TestHostResult, error) {
	config := sshPkg.HostConfig{
		Host: opts.Host, Port: opts.Port, User: opts.User,
		AuthMethod: opts.AuthMethod, KeyPath: opts.KeyPath,
	}
	connOK, progOK, msg := sshPkg.TestConnection(config, opts.Secret, program)
	return &TestHostResult{
		ConnectionOK: connOK,
		ProgramOK:    progOK,
		Message:      msg,
	}, nil
}

func (api *SessionAPI) GetRemoteDirInfo(hostID string, dir string) (*DirInfo, error) {
	client, err := api.hostManager.GetClient(hostID)
	if err != nil {
		return &DirInfo{DefaultBranch: "main", Branches: []string{}}, nil
	}
	defer api.hostManager.ReleaseClient(hostID)

	out, err := client.RunCommand(fmt.Sprintf("cd %s && git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@'", shellEscape(dir)))
	defaultBranch := "main"
	if err == nil {
		if b := strings.TrimSpace(out); b != "" {
			defaultBranch = b
		}
	}

	out, err = client.RunCommand(fmt.Sprintf("cd %s && git branch -a --sort=-committerdate --format='%%(refname:short)' 2>/dev/null | head -100", shellEscape(dir)))
	branches := []string{}
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line != "" {
				branches = append(branches, line)
			}
		}
	}

	return &DirInfo{DefaultBranch: defaultBranch, Branches: branches}, nil
}

func (api *SessionAPI) SearchRemoteBranches(hostID string, dir string, filter string) ([]string, error) {
	client, err := api.hostManager.GetClient(hostID)
	if err != nil {
		return []string{}, nil
	}
	defer api.hostManager.ReleaseClient(hostID)

	cmd := fmt.Sprintf("cd %s && git branch -a --sort=-committerdate --format='%%(refname:short)' 2>/dev/null", shellEscape(dir))
	if filter != "" {
		cmd += fmt.Sprintf(" | grep -i %s", shellEscape(filter))
	}
	cmd += " | head -100"

	out, err := client.RunCommand(cmd)
	if err != nil {
		return []string{}, nil
	}
	branches := []string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func generateAppUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// saveInstancesLocked persists all instances to disk. Must be called with api.mu held.
// Only writes if instances have been modified (dirty flag).
func (api *SessionAPI) saveInstancesLocked() {
	if !api.dirty {
		return
	}
	instances := make([]*session.Instance, 0, len(api.instances))
	for _, inst := range api.instances {
		instances = append(instances, inst)
	}
	if err := api.storage.SaveInstances(instances); err != nil {
		log.ErrorLog.Printf("failed to save instances: %v", err)
	}
}
