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
 * Register a DefinitionProvider for all supported languages.
 * Call once when the Monaco editor mounts in scope mode.
 * Returns a disposable to clean up on unmount.
 */
export function registerDefinitionProvider(
  monaco: typeof Monaco,
  sessionId: string
): Monaco.IDisposable {
  const provider: Monaco.languages.DefinitionProvider = {
    provideDefinition: async (
      model,
      position
    ): Promise<Monaco.languages.Definition | null> => {
      const word = model.getWordAtPosition(position);
      if (!word) return null;

      let defs: SymbolDefinition[];
      try {
        defs = await api().LookupSymbol(sessionId, word.word);
      } catch {
        return null;
      }

      if (!defs || defs.length === 0) return null;

      const locations: Monaco.languages.Location[] = [];
      for (const def of defs) {
        const uri = monaco.Uri.file(def.path);
        let targetModel = monaco.editor.getModel(uri);
        if (!targetModel) {
          try {
            const contents = await api().ReadFile(sessionId, def.path);
            const lang = detectLanguage(def.path);
            targetModel = monaco.editor.createModel(contents, lang, uri);
          } catch {
            continue;
          }
        }

        locations.push({
          uri,
          range: new monaco.Range(def.line, 1, def.line, 1),
        });
      }

      return locations.length > 0 ? locations : null;
    },
  };

  const disposables = SUPPORTED_LANGUAGES.map((lang) =>
    monaco.languages.registerDefinitionProvider(lang, provider)
  );

  return {
    dispose: () => disposables.forEach((d) => d.dispose()),
  };
}

