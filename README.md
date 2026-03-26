# Claude Squad IDE

A native desktop app for running multiple AI coding agents in parallel without them stepping on each other's code.

Works with [Claude Code](https://github.com/anthropics/claude-code), [Codex](https://github.com/openai/codex), [Gemini](https://github.com/google-gemini/gemini-cli), [Aider](https://github.com/Aider-AI/aider), and other terminal-based agents.

> *Originally forked from [smtg-ai/claude-squad](https://github.com/smtg-ai/claude-squad), licensed under [AGPL-3.0](LICENSE.md).*

### The Problem

When you're working across multiple repos or features, running several AI agents at once gets messy fast. They clobber each other's changes, you lose track of which agent is doing what, and context-switching between terminal windows burns time.

### How Claude Squad IDE Helps

**Manage all your sessions in one place.** Each agent session lives in its own panel. Split panes to watch several at once, or focus on one at a time. No more juggling terminal tabs.

**Isolated git worktrees per session.** Every new session gets its own git worktree branched from the ref you choose. Agents work in parallel on the same repo without merge conflicts. This extends to submodules too — if your repo has git submodules, each session's worktree initializes them independently so cross-repo features stay isolated.

**Session scope mode.** Enter a session's scope to browse and edit files in that specific worktree. See exactly what the agent changed, make manual edits, then switch back to the overview.

**Remote sessions.** Spin up agent sessions on another machine over SSH. Useful when you need more compute or want to keep heavy workloads off your laptop.

**Hands-off background work.** Auto-accept mode lets agents run unattended. Check back when they're done.

### Installation

**macOS DMG** (recommended): Download the `.dmg` from [Releases](https://github.com/jeffadamsc/claude-squad-ide/releases), open it, and drag the app to Applications. Then create a CLI symlink:

```bash
ln -sf /Applications/claude-squad.app/Contents/MacOS/cs ~/.local/bin/cs
```

**Build from source:**

```bash
git clone https://github.com/jeffadamsc/claude-squad-ide.git
cd claude-squad-ide
wails build
```

Install the built app:

```bash
cp -R build/bin/claude-squad.app /Applications/
ln -sf /Applications/claude-squad.app/Contents/MacOS/cs ~/.local/bin/cs
```

**Build a DMG installer:**

```bash
./scripts/build-dmg.sh
```

Output: `build/bin/Claude Squad.dmg`

### Prerequisites

- [Go](https://go.dev/dl/) 1.21+ (to build from source)
- [Wails](https://wails.io/docs/gettingstarted/installation) v2 (to build from source)
- [tmux](https://github.com/tmux/tmux/wiki/Installing) (used under the hood for agent terminal sessions)

### Usage

```
Usage:
  cs [flags]
  cs [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  debug       Print debug information like config paths
  help        Help about any command
  reset       Reset all stored instances
  version     Print the version number of claude-squad

Flags:
  -y, --autoyes          [experimental] If enabled, all instances will automatically accept prompts
  -h, --help             help for claude-squad
  -p, --program string   Program to run in new instances (e.g. 'aider --model ollama_chat/gemma3:1b')
```

Launch the app from the command line, from Spotlight, or from the Applications folder:

```bash
cs
```

The default program is `claude`. You can override it with `-p` or by configuring profiles (see below).

**Using other AI assistants:**
- Codex: `cs -p "codex"` (set `OPENAI_API_KEY` first)
- Aider: `cs -p "aider ..."`
- Gemini: `cs -p "gemini"`

### Keyboard Shortcuts

All shortcuts use **Cmd+Shift** (macOS) or **Ctrl+Shift** (Linux/Windows) as the modifier.

| Shortcut | Action |
|----------|--------|
| `N` | New session |
| `\` | Split pane vertically |
| `-` | Split pane horizontally |
| `W` | Close focused pane |
| `Arrow keys` | Navigate between panes |
| `J` / `K` | Move down / up in the sidebar session list |
| `Enter` | Open selected session in the focused pane |
| `D` | Kill (delete) session |
| `P` | Push changes |
| `R` | Pause / resume session |
| `B` | Toggle sidebar visibility |
| `Q` | Quit |

### Configuration

Claude Squad IDE stores its configuration in `~/.claude-squad/config.json`. You can find the exact path by running `cs debug`.

#### Profiles

Profiles let you define multiple named program configurations and switch between them when creating a new session. The new session dialog shows a profile dropdown when more than one profile is defined.

```json
{
  "default_program": "claude",
  "profiles": [
    { "name": "claude", "program": "claude" },
    { "name": "codex", "program": "codex" },
    { "name": "aider", "program": "aider --model ollama_chat/gemma3:1b" }
  ]
}
```

| Field     | Description                                              |
|-----------|----------------------------------------------------------|
| `name`    | Display name shown in the profile picker                 |
| `program` | Shell command used to launch the agent for that profile  |

If no profiles are defined, Claude Squad IDE uses `default_program` directly as the launch command (the default is `claude`).

### How It Works

1. **tmux** to create isolated terminal sessions for each agent
2. **git worktrees** to isolate codebases so each session works on its own branch
3. A native GUI (Wails + React) with embedded terminal panes for viewing and interacting with sessions

### License

[AGPL-3.0](LICENSE.md)
