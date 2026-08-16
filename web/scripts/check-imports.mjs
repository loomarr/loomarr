import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const sourceRoots = [
  "../apps/web/src/",
  "../packages/api/src/",
  "../packages/core/src/",
  "../packages/fixtures/src/",
  "../packages/tokens/src/",
].map((path) => fileURLToPath(new URL(path, import.meta.url)));

// These files are useful catalogs for tests, stories, and editor tooling. A production module
// importing one makes its dependency surface ambiguous and, for a value import, can pull an entire
// domain back across a route boundary (frontend-design §4.4).
const CATALOG_IMPORTS = new Set([
  "@/auth",
  "@/channels",
  "@/components/loomarr",
  "@/components/ui",
  "@/events",
  "@/filler",
  "@/help",
  "@/lib",
  "@/palette",
  "@/people",
  "@/queue",
  "@/settings",
  "@/suggest",
  "@/wizard",
  "@loomarr/api",
  "@loomarr/core",
]);

const isToolingFile = (path) =>
  path.includes("/test/") ||
  path.endsWith(".test.ts") ||
  path.endsWith(".test.tsx") ||
  path.endsWith(".stories.ts") ||
  path.endsWith(".stories.tsx");

const sourceFiles = (directory) =>
  readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return sourceFiles(path);
    return /\.(?:ts|tsx)$/.test(entry.name) && !isToolingFile(path) ? [path] : [];
  });

// Remove comments while preserving every character position. This keeps an import-looking code
// sample in a comment from becoming a false positive and lets diagnostics point at the original
// line and column without needing a second parser dependency.
const withoutComments = (source) => {
  let result = "";
  let state = "code";
  for (let i = 0; i < source.length; i += 1) {
    const char = source[i];
    const next = source[i + 1];
    if (state === "line") {
      if (char === "\n") {
        state = "code";
        result += char;
      } else result += " ";
    } else if (state === "block") {
      if (char === "*" && next === "/") {
        result += "  ";
        state = "code";
        i += 1;
      } else result += char === "\n" ? char : " ";
    } else if (state === "single" || state === "double" || state === "template") {
      result += char;
      if (char === "\\" && next) {
        result += next;
        i += 1;
      } else if (
        (state === "single" && char === "'") ||
        (state === "double" && char === '"') ||
        (state === "template" && char === "`")
      ) {
        state = "code";
      }
    } else if (char === "/" && next === "/") {
      result += "  ";
      state = "line";
      i += 1;
    } else if (char === "/" && next === "*") {
      result += "  ";
      state = "block";
      i += 1;
    } else {
      result += char;
      if (char === "'") state = "single";
      else if (char === '"') state = "double";
      else if (char === "`") state = "template";
    }
  }
  return result;
};

const findCatalogImports = (source) => {
  const searchable = withoutComments(source);
  const violations = [];
  const patterns = [
    /(?:^|[;\n])\s*(?:import|export)\s+(?:type\s+)?(?:[^"'`;]*?\s+from\s+)?(["'])([^"']+)\1/gm,
    /\bimport\s*\(\s*(["'])([^"']+)\1\s*\)/g,
  ];
  for (const pattern of patterns) {
    for (const match of searchable.matchAll(pattern)) {
      const importPath = match[2];
      if (!importPath || !CATALOG_IMPORTS.has(importPath)) continue;
      const offset = match.index + match[0].lastIndexOf(importPath);
      const before = source.slice(0, offset);
      const lineStart = before.lastIndexOf("\n") + 1;
      violations.push({
        importPath,
        line: before.split("\n").length,
        column: offset - lineStart + 1,
        offset,
      });
    }
  }
  return violations.sort((a, b) => a.offset - b.offset).map(({ offset: _, ...violation }) => violation);
};

const checkImports = (roots = sourceRoots) =>
  roots.flatMap((root) =>
    sourceFiles(root).flatMap((file) =>
      findCatalogImports(readFileSync(file, "utf8")).map((violation) => ({ file, ...violation })),
    ),
  );

const isMain = process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1];
if (isMain) {
  const violations = checkImports();
  if (violations.length > 0) {
    console.error("Production imports must name the nearest module instead of a catalog root:");
    for (const violation of violations) {
      console.error(
        `  ${violation.file}:${violation.line}:${violation.column} imports ${violation.importPath}`,
      );
    }
    console.error("Use a component, endpoint, model, or core-module subpath (frontend-design §4.4).");
    process.exitCode = 1;
  } else {
    console.log("import-boundaries: clean");
  }
}

export { checkImports, findCatalogImports };
