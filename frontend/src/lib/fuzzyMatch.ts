export interface FuzzyResult {
  path: string;
  score: number;
  matches: number[]; // indices of matched characters in the basename
}

/**
 * Fuzzy-match a query against a list of file paths.
 * Returns results sorted by score (highest first), capped at `limit`.
 */
export function fuzzyMatch(
  query: string,
  paths: string[],
  limit = 20
): FuzzyResult[] {
  if (!query)
    return paths
      .slice(0, limit)
      .map((p) => ({ path: p, score: 0, matches: [] }));

  const lowerQuery = query.toLowerCase();
  const results: FuzzyResult[] = [];

  for (const path of paths) {
    const basename = path.slice(path.lastIndexOf("/") + 1);
    const lowerBasename = basename.toLowerCase();

    // Try matching against basename first (higher score)
    const basenameResult = matchString(lowerQuery, lowerBasename);
    if (basenameResult) {
      results.push({
        path,
        score: basenameResult.score + 100,
        matches: basenameResult.indices,
      });
      continue;
    }

    // Fall back to matching against full path
    const lowerPath = path.toLowerCase();
    const pathResult = matchString(lowerQuery, lowerPath);
    if (pathResult) {
      const basenameStart = path.lastIndexOf("/") + 1;
      const basenameMatches = pathResult.indices
        .filter((i) => i >= basenameStart)
        .map((i) => i - basenameStart);
      results.push({
        path,
        score: pathResult.score,
        matches: basenameMatches,
      });
    }
  }

  results.sort((a, b) => b.score - a.score);
  return results.slice(0, limit);
}

function matchString(
  query: string,
  target: string
): { score: number; indices: number[] } | null {
  let qi = 0;
  let score = 0;
  const indices: number[] = [];
  let lastMatchIdx = -2;

  for (let ti = 0; ti < target.length && qi < query.length; ti++) {
    if (target[ti] === query[qi]) {
      indices.push(ti);
      if (ti === lastMatchIdx + 1) {
        score += 10; // consecutive match bonus
      } else {
        score += 5;
      }
      if (ti === 0) {
        score += 15; // start-of-string bonus
      }
      if (ti > 0 && "/.-_".includes(target[ti - 1])) {
        score += 10; // after separator bonus
      }
      lastMatchIdx = ti;
      qi++;
    }
  }

  if (qi < query.length) return null;
  return { score, indices };
}
