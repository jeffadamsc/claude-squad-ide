# Claude Squad IDE

## Building & Installing

This is a **Wails** application. Use `wails build` (not plain `go build`) to compile
the frontend, embed assets, and produce a macOS `.app` bundle:

```sh
wails build -skipbindings
```

The `-skipbindings` flag is required because the tree-sitter indexer uses CGO, and
Wails' binding generator doesn't support CGO dependencies. The TypeScript bindings
are pre-generated and checked into the repo.

The output is `build/bin/claude-squad.app`. Install it and create a CLI symlink:

```sh
cp -R build/bin/claude-squad.app /Applications/
ln -sf /Applications/claude-squad.app/Contents/MacOS/cs ~/.local/bin/cs
```

The `cs` binary is then available on PATH at `~/.local/bin/cs`.

### DMG Installer

To create a `.dmg` for distribution:

```sh
./scripts/build-dmg.sh
```

Output: `build/bin/Claude Squad.dmg`

### Development

For live-reload during development:

```sh
wails dev -skipbindings
```

Do NOT use `go build` or `go install` directly — they skip frontend compilation
and macOS app bundling.

### Regenerating Bindings

If you add new exported methods to `SessionAPI` that need frontend access, temporarily
remove the tree-sitter imports from `app/indexer_languages.go`, run `wails generate module`,
then restore the imports. The bindings in `frontend/wailsjs/go/` are checked into git.
