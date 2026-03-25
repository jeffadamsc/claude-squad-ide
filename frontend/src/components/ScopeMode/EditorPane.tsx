import { useRef, useEffect, useCallback } from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import { useSessionStore } from "../../store/sessionStore";
import { EditorTabBar } from "./EditorTabBar";
import { catppuccinMocha } from "../../lib/monacoTheme";
import { api } from "../../lib/wails";

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

  const activeFile = openEditorFiles.find(
    (f) => f.path === activeEditorFile
  );

  const handleMount: OnMount = (_editor, monaco) => {
    monaco.editor.defineTheme("catppuccin-mocha", catppuccinMocha);
    monaco.editor.setTheme("catppuccin-mocha");
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

  // Flush pending save on unmount
  useEffect(() => {
    return () => {
      clearTimeout(saveTimerRef.current);
      if (pendingSaveRef.current) {
        const { path, contents } = pendingSaveRef.current;
        api()
          .WriteFile(sessionId, path, contents)
          .catch(console.error);
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
          }}
        />
      </div>
    </div>
  );
}
