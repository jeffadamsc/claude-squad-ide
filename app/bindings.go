package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"claude-squad/config"
	"claude-squad/log"
	ptyPkg "claude-squad/pty"
	"claude-squad/session"
	"claude-squad/session/git"
	sshPkg "claude-squad/ssh"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
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

type DiffFileResult struct {
	Path       string `json:"path"`
	OldContent string `json:"oldContent"`
	NewContent string `json:"newContent"`
	Status     string `json:"status"`
	Submodule  string `json:"submodule"`
}

type SessionAPI struct {
	mu            sync.RWMutex
	ctx           context.Context // Wails app context for native dialogs
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
	indexers      map[string]*SessionIndexer
}

func NewSessionAPI(opts SessionAPIOptions) (*SessionAPI, error) {
	mgr := ptyPkg.NewManager()

	configDir, _ := config.GetConfigDir()
	hostStore := sshPkg.NewHostStore(filepath.Join(configDir, "hosts.json"))
	keychainStore := sshPkg.NewKeychainStore("com.claude-squad")
	hostMgr := sshPkg.NewHostManager(hostStore, keychainStore)

	sshRegistry := sshPkg.NewDynamicSSHRegistry(hostMgr)
	composite := ptyPkg.NewCompositeRegistry(mgr, sshRegistry)
	compositeResizer := ptyPkg.NewCompositeResizer(mgr, sshRegistry)
	ws := ptyPkg.NewWebSocketServer(composite, compositeResizer)

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
		instances:     make(map[string]*session.Instance),
		storage:       storage,
		ptyManager:    mgr,
		wsServer:      ws,
		wsPort:        port,
		cfg:           cfg,
		hostManager:   hostMgr,
		hostStore:     hostStore,
		keychainStore: keychainStore,
		indexers:      make(map[string]*SessionIndexer),
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

// SetContext stores the Wails application context needed for native dialogs.
func (api *SessionAPI) SetContext(ctx context.Context) {
	api.ctx = ctx
}

// SelectFile opens a native file dialog starting at the given directory.
func (api *SessionAPI) SelectFile(startDir string) (string, error) {
	if api.ctx == nil {
		return "", fmt.Errorf("application context not set")
	}
	// Expand ~ to home directory
	if strings.HasPrefix(startDir, "~/") {
		home, _ := os.UserHomeDir()
		startDir = filepath.Join(home, startDir[2:])
	}
	path, err := wailsRuntime.OpenFileDialog(api.ctx, wailsRuntime.OpenDialogOptions{
		DefaultDirectory: startDir,
		Title:            "Select SSH Key File",
	})
	if err != nil {
		return "", err
	}
	return path, nil
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

	log.InfoLog.Printf("CreateSession: title=%q path=%q hostID=%q", opts.Title, opts.Path, opts.HostID)

	program := opts.Program
	if program == "" {
		program = api.cfg.DefaultProgram
	}

	var pm session.ProcessManager = api.ptyManager
	var gitExec git.CommandExecutor
	if opts.HostID != "" {
		client, err := api.hostManager.GetClient(opts.HostID)
		if err != nil {
			return nil, fmt.Errorf("connect to remote host: %w", err)
		}
		// Resolve ~ to absolute path so all downstream commands work.
		opts.Path, err = resolveTilde(client, opts.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve remote path: %w", err)
		}

		sshPM, err := api.hostManager.GetProcessManager(opts.HostID)
		if err != nil {
			return nil, fmt.Errorf("get ssh process manager: %w", err)
		}
		pm = sshPM
		gitExec = &git.RemoteExecutor{
			RunCmd: func(cmd string) (string, error) {
				return client.RunCommand("bash -lc " + sshPkg.ShellEscape(cmd))
			},
		}
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
		GitExecutor:    gitExec,
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
		client, err := api.hostManager.GetClient(inst.HostID)
		if err != nil {
			return "", fmt.Errorf("reconnect to remote host: %w", err)
		}
		pm, err := api.hostManager.GetProcessManager(inst.HostID)
		if err != nil {
			return "", fmt.Errorf("get ssh process manager: %w", err)
		}
		inst.SetProcessManager(pm)
		inst.SetGitExecutor(&git.RemoteExecutor{
			RunCmd: func(cmd string) (string, error) {
				return client.RunCommand("bash -lc " + sshPkg.ShellEscape(cmd))
			},
		})
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
	log.InfoLog.Printf("StartSession called for id=%q", id)
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
		log.ErrorLog.Printf("StartSession failed for %q: %v", id, err)
		// Reset status so the UI doesn't show it as permanently loading
		api.mu.Lock()
		inst.SetStatus(session.Ready)
		api.mu.Unlock()
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
		log.ErrorLog.Printf("kill session %q cleanup error (session will still be removed): %v", id, err)
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
		hasPrompt := inst.HasPrompt()

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

	for _, idx := range api.indexers {
		idx.Stop()
	}

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
	LastPath   string `json:"lastPath"`
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
			KeyPath: h.KeyPath, LastPath: h.LastPath,
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

	dir, _ = resolveTilde(client, dir)

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

	dir, _ = resolveTilde(client, dir)

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

type RemoteDirEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
}

// DirectoryEntry represents a file or directory in a session's worktree.
type DirectoryEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

// resolveSessionPath resolves a relative path within a session's worktree
// and ensures it doesn't escape via path traversal.
func (api *SessionAPI) resolveSessionPath(inst *session.Instance, relPath string) (string, string, error) {
	worktree := inst.GetWorktreePath()
	if worktree == "" {
		worktree = inst.Path
	}
	cleaned := filepath.Clean("/" + relPath)
	absPath := filepath.Join(worktree, cleaned)
	if !strings.HasPrefix(absPath, worktree) {
		return "", "", fmt.Errorf("path outside worktree: %s", relPath)
	}
	return worktree, absPath, nil
}

// ListDirectory lists files and directories in a session's worktree path.
func (api *SessionAPI) ListDirectory(sessionID string, dirPath string) ([]DirectoryEntry, error) {
	api.mu.RLock()
	inst, ok := api.instances[sessionID]
	api.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	if inst.HostID != "" {
		return api.listDirectoryRemote(inst, dirPath)
	}
	return api.listDirectoryLocal(inst, dirPath)
}

func (api *SessionAPI) listDirectoryLocal(inst *session.Instance, dirPath string) ([]DirectoryEntry, error) {
	worktree, absPath, err := api.resolveSessionPath(inst, dirPath)
	if err != nil {
		return nil, err
	}

	dirEntries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	// Collect names for git check-ignore
	var names []string
	for _, e := range dirEntries {
		name := e.Name()
		if name == ".git" {
			continue
		}
		names = append(names, name)
	}

	// Use git check-ignore to filter gitignored files
	ignored := make(map[string]bool)
	if len(names) > 0 {
		cmd := exec.Command("git", "check-ignore", "--stdin")
		cmd.Dir = worktree
		cmd.Stdin = bytes.NewBufferString(strings.Join(func() []string {
			// Build paths relative to worktree for git check-ignore
			relDir := strings.TrimPrefix(absPath, worktree)
			relDir = strings.TrimPrefix(relDir, "/")
			var paths []string
			for _, n := range names {
				if relDir == "" {
					paths = append(paths, n)
				} else {
					paths = append(paths, relDir+"/"+n)
				}
			}
			return paths
		}(), "\n") + "\n")
		out, _ := cmd.Output() // exit code 1 means no matches, which is fine
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line != "" {
				ignored[filepath.Base(line)] = true
			}
		}
	}

	var entries []DirectoryEntry
	for _, e := range dirEntries {
		name := e.Name()
		if name == ".git" || ignored[name] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		entryPath := dirPath
		if entryPath == "." {
			entryPath = name
		} else {
			entryPath = entryPath + "/" + name
		}
		entries = append(entries, DirectoryEntry{
			Name:  name,
			Path:  entryPath,
			IsDir: e.IsDir(),
			Size:  info.Size(),
		})
	}
	return entries, nil
}

func (api *SessionAPI) listDirectoryRemote(inst *session.Instance, dirPath string) ([]DirectoryEntry, error) {
	_, absPath, err := api.resolveSessionPath(inst, dirPath)
	if err != nil {
		return nil, err
	}
	worktree := inst.GetWorktreePath()
	if worktree == "" {
		worktree = inst.Path
	}

	client, err := api.hostManager.GetClient(inst.HostID)
	if err != nil {
		return nil, fmt.Errorf("connect to host: %w", err)
	}
	defer api.hostManager.ReleaseClient(inst.HostID)

	// List all entries (files and dirs)
	out, err := client.RunCommand(fmt.Sprintf("ls -1pA %s 2>/dev/null", shellEscape(absPath)))
	if err != nil {
		return nil, fmt.Errorf("list directory: %w", err)
	}

	var rawNames []string
	var rawIsDir []bool
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		if line == ".git" || line == ".git/" {
			continue
		}
		isDir := strings.HasSuffix(line, "/")
		name := strings.TrimSuffix(line, "/")
		if name != "" {
			rawNames = append(rawNames, name)
			rawIsDir = append(rawIsDir, isDir)
		}
	}

	// Filter with git check-ignore on remote
	ignored := make(map[string]bool)
	if len(rawNames) > 0 {
		relDir := strings.TrimPrefix(absPath, worktree)
		relDir = strings.TrimPrefix(relDir, "/")
		var paths []string
		for _, n := range rawNames {
			if relDir == "" {
				paths = append(paths, n)
			} else {
				paths = append(paths, relDir+"/"+n)
			}
		}
		ignoreInput := strings.Join(paths, "\n") + "\n"
		ignoreOut, _ := client.RunCommand(fmt.Sprintf("cd %s && echo %s | base64 -d | git check-ignore --stdin 2>/dev/null",
			shellEscape(worktree), shellEscape(base64.StdEncoding.EncodeToString([]byte(ignoreInput)))))
		for _, line := range strings.Split(strings.TrimSpace(ignoreOut), "\n") {
			if line != "" {
				ignored[filepath.Base(line)] = true
			}
		}
	}

	var entries []DirectoryEntry
	for i, name := range rawNames {
		if ignored[name] {
			continue
		}
		entryPath := dirPath
		if entryPath == "." {
			entryPath = name
		} else {
			entryPath = entryPath + "/" + name
		}
		entries = append(entries, DirectoryEntry{
			Name:  name,
			Path:  entryPath,
			IsDir: rawIsDir[i],
		})
	}
	return entries, nil
}

// ReadFile reads a file from a session's worktree.
func (api *SessionAPI) ReadFile(sessionID string, filePath string) (string, error) {
	api.mu.RLock()
	inst, ok := api.instances[sessionID]
	api.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("session %s not found", sessionID)
	}

	_, absPath, err := api.resolveSessionPath(inst, filePath)
	if err != nil {
		return "", err
	}

	if inst.HostID != "" {
		client, err := api.hostManager.GetClient(inst.HostID)
		if err != nil {
			return "", fmt.Errorf("connect to host: %w", err)
		}
		defer api.hostManager.ReleaseClient(inst.HostID)
		out, err := client.RunCommand(fmt.Sprintf("cat %s", shellEscape(absPath)))
		if err != nil {
			return "", fmt.Errorf("read remote file: %w", err)
		}
		return out, nil
	}

	// Check file size (5MB limit)
	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	if info.Size() > 5*1024*1024 {
		return "", fmt.Errorf("file too large (%d bytes)", info.Size())
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	// Reject binary files (null bytes in first 512 bytes)
	check := data
	if len(check) > 512 {
		check = check[:512]
	}
	for _, b := range check {
		if b == 0 {
			return "", fmt.Errorf("binary file not supported")
		}
	}
	return string(data), nil
}

// WriteFile writes contents to a file in a session's worktree.
func (api *SessionAPI) WriteFile(sessionID string, filePath string, contents string) error {
	api.mu.RLock()
	inst, ok := api.instances[sessionID]
	api.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}

	_, absPath, err := api.resolveSessionPath(inst, filePath)
	if err != nil {
		return err
	}

	if inst.HostID != "" {
		client, err := api.hostManager.GetClient(inst.HostID)
		if err != nil {
			return fmt.Errorf("connect to host: %w", err)
		}
		defer api.hostManager.ReleaseClient(inst.HostID)
		encoded := base64.StdEncoding.EncodeToString([]byte(contents))
		_, err = client.RunCommand(fmt.Sprintf("echo %s | base64 -d > %s", shellEscape(encoded), shellEscape(absPath)))
		if err != nil {
			return fmt.Errorf("write remote file: %w", err)
		}
		return nil
	}

	if err := os.WriteFile(absPath, []byte(contents), 0644); err != nil {
		return err
	}

	// Trigger index refresh after file write
	api.mu.RLock()
	if idx, ok := api.indexers[sessionID]; ok {
		idx.Refresh()
	}
	api.mu.RUnlock()
	return nil
}

// ListFiles returns all git-tracked files in the session's worktree.
func (api *SessionAPI) ListFiles(sessionID string) ([]string, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	// Use indexer cache if available and non-empty
	if idx, ok := api.indexers[sessionID]; ok {
		files := idx.Files()
		if len(files) > 0 {
			log.InfoLog.Printf("ListFiles(%s): returning %d files from indexer cache", sessionID, len(files))
			return files, nil
		}
		log.InfoLog.Printf("ListFiles(%s): indexer cache empty, falling through", sessionID)
	}

	inst, ok := api.instances[sessionID]
	if !ok {
		log.ErrorLog.Printf("ListFiles(%s): session not found in instances", sessionID)
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	if inst.HostID != "" {
		return api.listFilesRemote(inst)
	}

	worktree := inst.GetWorktreePath()
	if worktree == "" {
		worktree = inst.Path
	}
	log.InfoLog.Printf("ListFiles(%s): running git ls-files in %s", sessionID, worktree)
	files, err := listFilesInWorktree(worktree)
	if err != nil {
		log.ErrorLog.Printf("ListFiles(%s): git ls-files error: %v", sessionID, err)
		return nil, err
	}
	log.InfoLog.Printf("ListFiles(%s): git ls-files returned %d files", sessionID, len(files))
	return files, err
}

// listFilesRemote runs git ls-files on a remote host via SSH.
func (api *SessionAPI) listFilesRemote(inst *session.Instance) ([]string, error) {
	client, err := api.hostManager.GetClient(inst.HostID)
	if err != nil {
		return nil, fmt.Errorf("SSH connect: %w", err)
	}
	defer api.hostManager.ReleaseClient(inst.HostID)

	path := shellEscape(inst.Path)
	out, err := client.RunCommand(fmt.Sprintf("cd %s && git ls-files", path))
	if err != nil {
		return nil, fmt.Errorf("remote git ls-files: %w", err)
	}
	raw := strings.TrimSpace(out)
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\n"), nil
}

// IndexSession starts or restarts the indexer for a scoped session.
func (api *SessionAPI) IndexSession(sessionID string) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Stop existing indexer if any
	if idx, ok := api.indexers[sessionID]; ok {
		idx.Stop()
	}

	inst, ok := api.instances[sessionID]
	if !ok {
		log.ErrorLog.Printf("IndexSession(%s): session not found", sessionID)
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Remote sessions: not yet supported for indexing
	if inst.HostID != "" {
		log.InfoLog.Printf("IndexSession(%s): remote session, skipping", sessionID)
		return nil
	}

	worktree := inst.GetWorktreePath()
	if worktree == "" {
		worktree = inst.Path
	}
	log.InfoLog.Printf("IndexSession(%s): starting indexer for worktree=%s", sessionID, worktree)

	idx := NewSessionIndexer(worktree)
	idx.Start()
	api.indexers[sessionID] = idx
	return nil
}

// StopIndexer stops the indexer for a session (called on scope exit).
func (api *SessionAPI) StopIndexer(sessionID string) {
	api.mu.Lock()
	defer api.mu.Unlock()
	if idx, ok := api.indexers[sessionID]; ok {
		idx.Stop()
		delete(api.indexers, sessionID)
	}
}

// LookupSymbol returns definitions for a symbol name in the scoped session.
func (api *SessionAPI) LookupSymbol(sessionID string, symbol string) ([]Definition, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	idx, ok := api.indexers[sessionID]
	if !ok {
		log.InfoLog.Printf("LookupSymbol(%s, %q): no indexer", sessionID, symbol)
		return nil, nil
	}
	defs := idx.Lookup(symbol)
	log.InfoLog.Printf("LookupSymbol(%s, %q): found %d definitions", sessionID, symbol, len(defs))
	return defs, nil
}

// GetAllSymbols returns the entire symbol table for a session.
// Used by the frontend to cache symbols locally for instant definition lookups.
func (api *SessionAPI) GetAllSymbols(sessionID string) (map[string][]Definition, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	idx, ok := api.indexers[sessionID]
	if !ok {
		return nil, nil
	}
	return idx.AllSymbols(), nil
}

// ListRemoteDir lists directories in a path on a remote host.
func (api *SessionAPI) ListRemoteDir(hostID string, dir string) ([]RemoteDirEntry, error) {
	client, err := api.hostManager.GetClient(hostID)
	if err != nil {
		return nil, fmt.Errorf("connect to host: %w", err)
	}
	defer api.hostManager.ReleaseClient(hostID)

	dir, err = resolveTilde(client, dir)
	if err != nil {
		return nil, err
	}

	// List only directories, one per line
	out, err := client.RunCommand(fmt.Sprintf("ls -1pA %s 2>/dev/null", shellEscape(dir)))
	if err != nil {
		return nil, fmt.Errorf("list directory: %w", err)
	}

	var entries []RemoteDirEntry
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, "/") {
			name := strings.TrimSuffix(line, "/")
			if name != "" {
				entries = append(entries, RemoteDirEntry{Name: name, IsDir: true})
			}
		}
	}
	return entries, nil
}

// CheckRemoteGitRepo checks if a path on a remote host is a git repository.
func (api *SessionAPI) CheckRemoteGitRepo(hostID string, dir string) (bool, error) {
	client, err := api.hostManager.GetClient(hostID)
	if err != nil {
		return false, fmt.Errorf("connect to host: %w", err)
	}
	defer api.hostManager.ReleaseClient(hostID)

	dir, err = resolveTilde(client, dir)
	if err != nil {
		return false, err
	}

	_, err = client.RunCommand(fmt.Sprintf("cd %s && git rev-parse --show-toplevel", shellEscape(dir)))
	return err == nil, nil
}

// SetHostLastPath saves the last-used path for a host.
func (api *SessionAPI) SetHostLastPath(hostID string, path string) error {
	config, err := api.hostStore.GetByID(hostID)
	if err != nil {
		return err
	}
	config.LastPath = path
	return api.hostStore.Update(config)
}

// resolveTilde resolves ~ or ~/... paths using the remote host's $HOME.
func resolveTilde(client *sshPkg.Client, dir string) (string, error) {
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		homeOut, err := client.RunCommand("echo $HOME")
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		home := strings.TrimSpace(homeOut)
		if dir == "~" {
			return home, nil
		}
		return home + dir[1:], nil
	}
	return dir, nil
}

func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// GetDiffFiles returns all changed files for a session, including submodules.
func (api *SessionAPI) GetDiffFiles(sessionID string) ([]DiffFileResult, error) {
	api.mu.RLock()
	inst, ok := api.instances[sessionID]
	api.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	gw, err := inst.GetGitWorktree()
	if err != nil {
		return nil, fmt.Errorf("get git worktree: %w", err)
	}
	if gw == nil {
		return nil, fmt.Errorf("session has no git worktree (in-place session)")
	}

	gitFiles, err := gw.GetDiffFilesWithSubmodules()
	if err != nil {
		return nil, fmt.Errorf("get diff files: %w", err)
	}

	results := make([]DiffFileResult, len(gitFiles))
	for i, f := range gitFiles {
		results[i] = DiffFileResult{
			Path:       f.Path,
			OldContent: f.OldContent,
			NewContent: f.NewContent,
			Status:     f.Status,
			Submodule:  f.Submodule,
		}
	}
	return results, nil
}

func generateAppUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// CreateFile creates a new empty file (or with provided contents) in a session's worktree.
func (api *SessionAPI) CreateFile(sessionID string, filePath string) error {
	api.mu.RLock()
	inst, ok := api.instances[sessionID]
	api.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}

	_, absPath, err := api.resolveSessionPath(inst, filePath)
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	// Fail if file already exists
	if _, err := os.Stat(absPath); err == nil {
		return fmt.Errorf("file already exists: %s", filePath)
	}

	if err := os.WriteFile(absPath, []byte{}, 0644); err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	api.mu.RLock()
	if idx, ok := api.indexers[sessionID]; ok {
		idx.Refresh()
	}
	api.mu.RUnlock()
	return nil
}

// CreateDirectory creates a new directory in a session's worktree.
func (api *SessionAPI) CreateDirectory(sessionID string, dirPath string) error {
	api.mu.RLock()
	inst, ok := api.instances[sessionID]
	api.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}

	_, absPath, err := api.resolveSessionPath(inst, dirPath)
	if err != nil {
		return err
	}

	if _, err := os.Stat(absPath); err == nil {
		return fmt.Errorf("path already exists: %s", dirPath)
	}

	if err := os.MkdirAll(absPath, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	return nil
}

// DeletePath deletes a file or directory in a session's worktree.
func (api *SessionAPI) DeletePath(sessionID string, targetPath string) error {
	api.mu.RLock()
	inst, ok := api.instances[sessionID]
	api.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}

	_, absPath, err := api.resolveSessionPath(inst, targetPath)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(absPath); err != nil {
		return fmt.Errorf("delete path: %w", err)
	}

	api.mu.RLock()
	if idx, ok := api.indexers[sessionID]; ok {
		idx.Refresh()
	}
	api.mu.RUnlock()
	return nil
}

// RenamePath renames a file or directory in a session's worktree.
func (api *SessionAPI) RenamePath(sessionID string, oldPath string, newPath string) error {
	api.mu.RLock()
	inst, ok := api.instances[sessionID]
	api.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}

	_, absOld, err := api.resolveSessionPath(inst, oldPath)
	if err != nil {
		return err
	}
	_, absNew, err := api.resolveSessionPath(inst, newPath)
	if err != nil {
		return err
	}

	// Ensure parent of destination exists
	if err := os.MkdirAll(filepath.Dir(absNew), 0755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	if err := os.Rename(absOld, absNew); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	api.mu.RLock()
	if idx, ok := api.indexers[sessionID]; ok {
		idx.Refresh()
	}
	api.mu.RUnlock()
	return nil
}

// CopyPath copies a file or directory in a session's worktree.
func (api *SessionAPI) CopyPath(sessionID string, srcPath string, destPath string) error {
	api.mu.RLock()
	inst, ok := api.instances[sessionID]
	api.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}

	_, absSrc, err := api.resolveSessionPath(inst, srcPath)
	if err != nil {
		return err
	}
	_, absDest, err := api.resolveSessionPath(inst, destPath)
	if err != nil {
		return err
	}

	// Ensure parent of destination exists
	if err := os.MkdirAll(filepath.Dir(absDest), 0755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	info, err := os.Stat(absSrc)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	if info.IsDir() {
		// Use cp -R for directory copy
		cmd := exec.Command("cp", "-R", absSrc, absDest)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("copy directory: %s: %w", string(out), err)
		}
	} else {
		data, err := os.ReadFile(absSrc)
		if err != nil {
			return fmt.Errorf("read source: %w", err)
		}
		if err := os.WriteFile(absDest, data, info.Mode()); err != nil {
			return fmt.Errorf("write destination: %w", err)
		}
	}

	api.mu.RLock()
	if idx, ok := api.indexers[sessionID]; ok {
		idx.Refresh()
	}
	api.mu.RUnlock()
	return nil
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
