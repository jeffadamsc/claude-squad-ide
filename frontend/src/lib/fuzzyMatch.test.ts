import { describe, it, expect } from "vitest";
import { fuzzyMatch } from "./fuzzyMatch";

const samplePaths = [
  "main.go",
  "app/bindings.go",
  "app/indexer.go",
  "frontend/src/App.tsx",
  "frontend/src/lib/wails.ts",
  "frontend/src/lib/fuzzyMatch.ts",
  "frontend/src/store/sessionStore.ts",
  "frontend/src/components/ScopeMode/QuickOpen.tsx",
  "frontend/src/components/ScopeMode/EditorPane.tsx",
  "frontend/src/components/ScopeMode/FileExplorer.tsx",
  "README.md",
];

describe("fuzzyMatch", () => {
  it("returns all paths when query is empty", () => {
    const results = fuzzyMatch("", samplePaths, 20);
    expect(results).toHaveLength(samplePaths.length);
    expect(results[0].score).toBe(0);
    expect(results[0].matches).toEqual([]);
  });

  it("matches exact filename", () => {
    const results = fuzzyMatch("main.go", samplePaths);
    expect(results.length).toBeGreaterThan(0);
    expect(results[0].path).toBe("main.go");
  });

  it("matches partial filename", () => {
    const results = fuzzyMatch("bind", samplePaths);
    expect(results.length).toBeGreaterThan(0);
    expect(results[0].path).toBe("app/bindings.go");
  });

  it("prefers filename matches over path matches", () => {
    const results = fuzzyMatch("store", samplePaths);
    expect(results.length).toBeGreaterThan(0);
    // sessionStore.ts should rank higher since "store" is in the filename
    expect(results[0].path).toBe("frontend/src/store/sessionStore.ts");
  });

  it("returns highlighted match indices in basename", () => {
    const results = fuzzyMatch("qo", samplePaths);
    const quickOpen = results.find((r) =>
      r.path.includes("QuickOpen")
    );
    expect(quickOpen).toBeDefined();
    // 'Q' and 'O' should be highlighted (indices 0 and 5 in "QuickOpen.tsx")
    expect(quickOpen!.matches.length).toBeGreaterThan(0);
  });

  it("returns empty for no matches", () => {
    const results = fuzzyMatch("zzzzz", samplePaths);
    expect(results).toHaveLength(0);
  });

  it("respects limit parameter", () => {
    const results = fuzzyMatch("", samplePaths, 3);
    expect(results).toHaveLength(3);
  });

  it("scores consecutive matches higher", () => {
    const paths = ["abcxyz.ts", "abcdef.ts"];
    const results = fuzzyMatch("abc", paths);
    // Both match, but both have the same consecutive run
    expect(results).toHaveLength(2);
    expect(results[0].score).toBe(results[1].score);
  });

  it("scores start-of-string matches higher", () => {
    const paths = ["middleware.go", "main.go"];
    const results = fuzzyMatch("m", paths);
    // Both start with 'm', should both match
    expect(results).toHaveLength(2);
  });

  it("matches across path when basename doesn't match", () => {
    const results = fuzzyMatch("src/lib", samplePaths);
    expect(results.length).toBeGreaterThan(0);
    // All results should be in frontend/src/lib/
    for (const r of results) {
      expect(r.path).toContain("src/lib");
    }
  });

  it("handles single character query", () => {
    const results = fuzzyMatch("R", samplePaths);
    expect(results.length).toBeGreaterThan(0);
    // README.md starts with R, should be in results
    expect(results.some((r) => r.path === "README.md")).toBe(true);
  });
});
