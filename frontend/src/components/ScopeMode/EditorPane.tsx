import { useRef, useEffect, useCallback } from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import { useSessionStore, detectLanguage } from "../../store/sessionStore";
import { EditorTabBar } from "./EditorTabBar";
import { catppuccinMocha } from "../../lib/monacoTheme";
import { api } from "../../lib/wails";
import { registerDefinitionProvider } from "../../lib/definitionProvider";

interface EditorPaneProps {
  sessionId: string;
}

export function EditorPane({ sessionId }: EditorPaneProps) {
  const openEditorFiles = useSessionStore((s) => s.openEditorFiles);
  const activeEditorFile = useSessionStore((s) => s.activeEditorFile);
  const updateEditorFileContents = useSessionStore(
    (s) => s.updateEditorFileContents
  );
  const saveTimerRef = useRef<ReturnType<typeof setTimeout>>();
  const pendingSaveRef = useRef<{ path: string; contents: string } | null>(
    null
  );
  const definitionProviderRef = useRef<{ dispose: () => void } | null>(null);

  const activeFile = openEditorFiles.find(
    (f) => f.path === activeEditorFile
  );

  const handleMount: OnMount = (editor, monaco) => {
    monaco.editor.defineTheme("catppuccin-mocha", catppuccinMocha);
    monaco.editor.setTheme("catppuccin-mocha");

    // Register definition provider for go-to-definition
    if (definitionProviderRef.current) {
      definitionProviderRef.current.dispose();
    }
    definitionProviderRef.current = registerDefinitionProvider(
      monaco,
      sessionId
    );

    // When Monaco navigates to a definition in another file, open it in tabs
    editor.onDidChangeModel(() => {
      const model = editor.getModel();
      if (model) {
        const filePath = model.uri.path.startsWith("/")
          ? model.uri.path.slice(1)
          : model.uri.path;
        const store = useSessionStore.getState();
        const existing = store.openEditorFiles.find(
          (f) => f.path === filePath
        );
        if (existing) {
          store.setActiveEditorFile(filePath);
        } else {
          // File opened via go-to-definition — add to editor tabs
          store.openEditorFile(
            filePath,
            model.getValue(),
            detectLanguage(filePath)
          );
        }
      }
    });
  };

  const flushSave = useCallback(() => {
    if (pendingSaveRef.current) {
      const { path, contents } = pendingSaveRef.current;
      api()
        .WriteFile(sessionId, path, contents)
        .catch(console.error);
      pendingSaveRef.current = null;
    }
  }, [sessionId]);

  const handleChange = useCallback(
    (value: string | undefined) => {
      if (!value || !activeFile) return;
      updateEditorFileContents(activeFile.path, value);

      clearTimeout(saveTimerRef.current);
      pendingSaveRef.current = { path: activeFile.path, contents: value };
      saveTimerRef.current = setTimeout(() => {
        flushSave();
      }, 500);
    },
    [activeFile, updateEditorFileContents, flushSave]
  );

  // Flush pending save and clean up definition provider on unmount
  useEffect(() => {
    return () => {
      clearTimeout(saveTimerRef.current);
      if (pendingSaveRef.current) {
        const { path, contents } = pendingSaveRef.current;
        api()
          .WriteFile(sessionId, path, contents)
          .catch(console.error);
      }
      if (definitionProviderRef.current) {
        definitionProviderRef.current.dispose();
        definitionProviderRef.current = null;
      }
    };
  }, [sessionId]);

  if (!activeFile) {
    return (
      <div
        style={{
          display: "flex",
          flexDirection: "column",
          height: "100%",
          background: "var(--mantle)",
        }}
      >
        <EditorTabBar />
        <div
          style={{
            flex: 1,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            color: "var(--overlay0)",
            fontSize: 13,
          }}
        >
          Select a file to edit
        </div>
      </div>
    );
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <EditorTabBar />
      <div style={{ flex: 1 }}>
        <Editor
          key={activeFile.path}
          defaultValue={activeFile.contents}
          language={activeFile.language}
          theme="catppuccin-mocha"
          onMount={handleMount}
          onChange={handleChange}
          options={{
            minimap: { enabled: false },
            fontSize: 13,
            fontFamily:
              "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
            lineNumbers: "on",
            scrollBeyondLastLine: false,
            automaticLayout: true,
            wordWrap: "off",
            bracketPairColorization: { enabled: true },
            padding: { top: 8 },
            gotoLocation: {
              multiple: "peek",
              multipleDefinitions: "peek",
            },
          }}
        />
      </div>
    </div>
  );
}
