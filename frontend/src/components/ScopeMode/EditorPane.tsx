import { useRef, useEffect, useCallback, useState } from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import type * as Monaco from "monaco-editor";
import { useSessionStore, detectLanguage } from "../../store/sessionStore";
import { EditorTabBar } from "./EditorTabBar";
import { catppuccinMocha } from "../../lib/monacoTheme";
import { api } from "../../lib/wails";
import {
  registerDefinitionProvider,
  registerDefinitionLink,
  getSymbolCache,
} from "../../lib/definitionProvider";
import { DiffViewer } from "./DiffViewer";
import { MarkdownPreview } from "./MarkdownPreview";

interface EditorPaneProps {
  sessionId: string;
}

export function EditorPane({ sessionId }: EditorPaneProps) {
  const openEditorFiles = useSessionStore((s) => s.openEditorFiles);
  const activeEditorFile = useSessionStore((s) => s.activeEditorFile);
  const updateEditorFileContents = useSessionStore(
    (s) => s.updateEditorFileContents
  );
  const editorFontSize = useSessionStore((s) => s.editorFontSize);
  const zoomEditorFont = useSessionStore((s) => s.zoomEditorFont);
  const monacoRef = useRef<Monaco.editor.IStandaloneCodeEditor | null>(null);
  const saveTimerRef = useRef<ReturnType<typeof setTimeout>>();
  const pendingSaveRef = useRef<{ path: string; contents: string } | null>(
    null
  );
  const definitionProviderRef = useRef<{ dispose: () => void } | null>(null);
  const editorOpenerRef = useRef<{ dispose: () => void } | null>(null);
  const definitionLinkRef = useRef<{ dispose: () => void } | null>(null);
  const [showMarkdownPreview, setShowMarkdownPreview] = useState(false);

  const activeFile = openEditorFiles.find(
    (f) => f.path === activeEditorFile
  );

  const isMarkdown = activeFile?.language === "markdown" && activeFile.type === "file";

  const handleMount: OnMount = (editor, monaco) => {
    monacoRef.current = editor;
    monaco.editor.defineTheme("catppuccin-mocha", catppuccinMocha);
    monaco.editor.setTheme("catppuccin-mocha");

    // Register definition provider for go-to-definition
    try {
      if (definitionProviderRef.current) {
        definitionProviderRef.current.dispose();
      }
      definitionProviderRef.current = registerDefinitionProvider(
        monaco,
        sessionId
      );
    } catch (err) {
      console.error("Failed to register definition provider:", err);
    }

    // Register custom editor opener so cross-file go-to-definition
    // opens files in our tab system instead of Monaco trying to switch models
    try {
      if (editorOpenerRef.current) {
        editorOpenerRef.current.dispose();
      }
      editorOpenerRef.current = monaco.editor.registerEditorOpener({
        openCodeEditor(
          _source: Monaco.editor.ICodeEditor,
          resource: Monaco.Uri,
          selectionOrPosition?: Monaco.IRange | Monaco.IPosition
        ): boolean {
          const filePath = resource.path.startsWith("/")
            ? resource.path.slice(1)
            : resource.path;
          const store = useSessionStore.getState();

          // Determine the target line/column
          let line = 1;
          let column = 1;
          if (selectionOrPosition) {
            if ("lineNumber" in selectionOrPosition) {
              line = selectionOrPosition.lineNumber;
              column = selectionOrPosition.column;
            } else if ("startLineNumber" in selectionOrPosition) {
              line = selectionOrPosition.startLineNumber;
              column = selectionOrPosition.startColumn;
            }
          }

          // Check if file is already open in a tab
          const existing = store.openEditorFiles.find(
            (f) => f.path === filePath
          );
          if (existing) {
            store.setActiveEditorFile(filePath);
            store.setPendingReveal({ line, column });
          } else {
            // Read and open the file in a new tab
            api()
              .ReadFile(sessionId, filePath)
              .then((contents) => {
                const s = useSessionStore.getState();
                s.openEditorFile(filePath, contents, detectLanguage(filePath));
                s.setPendingReveal({ line, column });
              })
              .catch(console.error);
          }
          return true; // we handled it
        },
      });
    } catch (err) {
      console.error("Failed to register editor opener:", err);
    }

    // Register manual Cmd+hover blue underline for definition links
    try {
      if (definitionLinkRef.current) {
        definitionLinkRef.current.dispose();
      }
      definitionLinkRef.current = registerDefinitionLink(
        monaco,
        editor,
        getSymbolCache
      );
    } catch (err) {
      console.error("Failed to register definition link:", err);
    }

    // Apply pending reveal (scroll to line) if there is one
    const reveal = useSessionStore.getState().pendingReveal;
    if (reveal) {
      editor.revealLineInCenter(reveal.line);
      editor.setPosition({ lineNumber: reveal.line, column: reveal.column });
      useSessionStore.getState().setPendingReveal(null);
    }

    // Auto-focus the editor so Cmd+hover and keyboard shortcuts work immediately
    editor.focus();
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

  // Sync font size to live Monaco editor instance
  useEffect(() => {
    if (monacoRef.current) {
      monacoRef.current.updateOptions({ fontSize: editorFontSize });
    }
  }, [editorFontSize]);

  // Handle Cmd+/Cmd- for font zoom
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey)) return;
      if (e.key === "=" || e.key === "+") {
        e.preventDefault();
        zoomEditorFont(1);
      } else if (e.key === "-") {
        e.preventDefault();
        zoomEditorFont(-1);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [zoomEditorFont]);

  // Reset markdown preview when switching files
  useEffect(() => {
    setShowMarkdownPreview(false);
  }, [activeEditorFile]);

  // Clean up definition provider, editor opener, and link when sessionId changes or on unmount
  useEffect(() => {
    return () => {
      clearTimeout(saveTimerRef.current);
      if (pendingSaveRef.current) {
        const { path, contents } = pendingSaveRef.current;
        api()
          .WriteFile(sessionId, path, contents)
          .catch(console.error);
        pendingSaveRef.current = null;
      }
      if (definitionProviderRef.current) {
        definitionProviderRef.current.dispose();
        definitionProviderRef.current = null;
      }
      if (editorOpenerRef.current) {
        editorOpenerRef.current.dispose();
        editorOpenerRef.current = null;
      }
      if (definitionLinkRef.current) {
        definitionLinkRef.current.dispose();
        definitionLinkRef.current = null;
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

  const isDiffTab = activeFile.type === "diff";

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <EditorTabBar />
      {isMarkdown && (
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 8,
            padding: "4px 12px",
            background: "var(--mantle)",
            borderBottom: "1px solid var(--surface0)",
            fontSize: 12,
          }}
        >
          <span
            onClick={() => setShowMarkdownPreview(false)}
            style={{
              cursor: "pointer",
              color: !showMarkdownPreview ? "var(--blue)" : "var(--overlay0)",
              fontWeight: !showMarkdownPreview ? 600 : 400,
            }}
          >
            Edit
          </span>
          <span style={{ color: "var(--surface1)" }}>|</span>
          <span
            onClick={() => setShowMarkdownPreview(true)}
            style={{
              cursor: "pointer",
              color: showMarkdownPreview ? "var(--blue)" : "var(--overlay0)",
              fontWeight: showMarkdownPreview ? 600 : 400,
            }}
          >
            Preview
          </span>
        </div>
      )}
      <div style={{ flex: 1, overflow: "hidden" }}>
        {isDiffTab ? (
          <DiffViewer sessionId={sessionId} />
        ) : isMarkdown && showMarkdownPreview ? (
          <MarkdownPreview content={activeFile.contents} fontSize={editorFontSize} />
        ) : (
          <Editor
            key={activeFile.path}
            defaultValue={activeFile.contents}
            language={activeFile.language}
            theme="catppuccin-mocha"
            onMount={handleMount}
            onChange={handleChange}
            options={{
              minimap: { enabled: false },
              fontSize: editorFontSize,
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
        )}
      </div>
    </div>
  );
}
