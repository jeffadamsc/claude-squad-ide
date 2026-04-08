import type * as Monaco from "monaco-editor";
import { api } from "./wails";
import type { SymbolDefinition } from "./wails";
import { detectLanguage } from "../store/sessionStore";

const SUPPORTED_LANGUAGES = [
  "go",
  "typescript",
  "javascript",
  "typescriptreact",
  "javascriptreact",
  "c",
  "cpp",
];

/**
 * Register a DefinitionProvider and a Cmd+hover underline for go-to-definition.
 * Fetches the full symbol table from the backend and caches it locally
 * so lookups are instant.
 *
 * Call once when the Monaco editor mounts in scope mode.
 * Returns a disposable to clean up on unmount.
 */
// Shared symbol cache — accessible by both the provider and the link decorator
let _symbolCache: Record<string, SymbolDefinition[]> | null = null;
export function getSymbolCache(): Record<string, SymbolDefinition[]> | null {
  return _symbolCache;
}

export function registerDefinitionProvider(
  monaco: typeof Monaco,
  sessionId: string
): Monaco.IDisposable {

  const loadSymbols = () => {
    api()
      .GetAllSymbols(sessionId)
      .then((symbols) => {
        if (symbols) _symbolCache = symbols;
      })
      .catch(console.error);
  };

  // Initial load
  loadSymbols();
  // Refresh every 15 seconds to pick up new symbols from background ctags runs
  const refreshInterval = setInterval(loadSymbols, 15000);

  const provider: Monaco.languages.DefinitionProvider = {
    provideDefinition(
      model,
      position
    ): Monaco.languages.Definition | null {
      const word = model.getWordAtPosition(position);
      if (!word) return null;

      const defs = _symbolCache?.[word.word];
      if (!defs || defs.length === 0) return null;

      return defs.map((def) => {
        const uri = monaco.Uri.file(def.path);
        if (!monaco.editor.getModel(uri)) {
          monaco.editor.createModel("", detectLanguage(def.path), uri);
        }
        return {
          uri,
          range: new monaco.Range(def.line, 1, def.line, 1),
        };
      });
    },
  };

  const disposables = SUPPORTED_LANGUAGES.map((lang) =>
    monaco.languages.registerDefinitionProvider(lang, provider)
  );

  return {
    dispose: () => {
      clearInterval(refreshInterval);
      disposables.forEach((d) => d.dispose());
      _symbolCache = null;
    },
  };
}

/**
 * Adds Cmd+hover blue underline + pointer cursor for symbols that have definitions.
 * Monaco's built-in definition link doesn't work reliably in standalone mode,
 * so we implement it manually with decorations and mouse events.
 */
export function registerDefinitionLink(
  monaco: typeof Monaco,
  editor: Monaco.editor.ICodeEditor,
  symbolCacheGetter: () => Record<string, SymbolDefinition[]> | null
): Monaco.IDisposable {
  let currentDecorations: string[] = [];
  let isMetaHeld = false;
  let lastMouseEvent: Monaco.editor.IEditorMouseEvent | null = null;

  const clearDecorations = () => {
    currentDecorations = editor.deltaDecorations(currentDecorations, []);
  };

  const applyLink = (position: Monaco.IPosition) => {
    const model = editor.getModel();
    if (!model) return;

    const word = model.getWordAtPosition(position);
    if (!word) {
      clearDecorations();
      return;
    }

    const cache = symbolCacheGetter();
    const defs = cache?.[word.word];
    if (!defs || defs.length === 0) {
      clearDecorations();
      return;
    }

    // Show underline on the word
    currentDecorations = editor.deltaDecorations(currentDecorations, [
      {
        range: new monaco.Range(
          position.lineNumber,
          word.startColumn,
          position.lineNumber,
          word.endColumn
        ),
        options: {
          inlineClassName: "definition-link",
        },
      },
    ]);
  };

  const updateLink = (e: Monaco.editor.IEditorMouseEvent) => {
    lastMouseEvent = e;

    if (!isMetaHeld) {
      clearDecorations();
      return;
    }

    const target = e.target;
    if (
      target.type !== monaco.editor.MouseTargetType.CONTENT_TEXT ||
      !target.position
    ) {
      clearDecorations();
      return;
    }

    applyLink(target.position);
  };

  const onMouseMove = editor.onMouseMove(updateLink);

  const onKeyDown = editor.onKeyDown((e) => {
    if (e.keyCode === monaco.KeyCode.Meta || e.keyCode === monaco.KeyCode.Ctrl) {
      isMetaHeld = true;
      // Re-evaluate at last known mouse position so hover-then-Cmd works
      if (
        lastMouseEvent?.target?.type === monaco.editor.MouseTargetType.CONTENT_TEXT &&
        lastMouseEvent.target.position
      ) {
        applyLink(lastMouseEvent.target.position);
      }
    }
  });

  const onKeyUp = editor.onKeyUp((e) => {
    if (e.keyCode === monaco.KeyCode.Meta || e.keyCode === monaco.KeyCode.Ctrl) {
      isMetaHeld = false;
      clearDecorations();
    }
  });

  const onBlur = editor.onDidBlurEditorWidget(() => {
    isMetaHeld = false;
    clearDecorations();
  });

  return {
    dispose: () => {
      clearDecorations();
      onMouseMove.dispose();
      onKeyDown.dispose();
      onKeyUp.dispose();
      onBlur.dispose();
    },
  };
}
