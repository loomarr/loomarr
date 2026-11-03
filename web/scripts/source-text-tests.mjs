// The web half of the source-text test ban (AGENTS.md, Tests; the Go half is
// internal/guard_test.go). A test asserts on behaviour, never on the text of Go or TypeScript
// source: a test that reads a source file and matches its contents passes while the behaviour it
// names is broken and fails on harmless refactors. Exercise the behaviour, or, where the intent is
// structural, parse the source and check the syntax tree.
//
// The detection is syntactic, over each test file's Babel AST:
//   - A read is a call to readFileSync or readFile (bare or as a member, e.g. fs.readFileSync).
//   - A read is a source read when its path names source: a string or template literal ending in a
//     source extension reaches it directly, through new URL(), join() or any other call, a
//     concatenation, a variable, an array, or a function parameter (bound to the values at the
//     function's call sites, or to the array a callback iterates).
//   - A read whose path the guard cannot resolve is a source read when the file also selects files
//     by a source extension (.endsWith(".tsx"), a /\.tsx?$/ test, or an extname comparison).
//   - A path built from mkdtemp or tmpdir, or naming a testdata directory, is test output or a
//     fixture, never repo source.
//   - A read assigned to a name the file hands to a parser (@babel/parser, oxc-parser, typescript,
//     acorn) is syntax-tree input, not text under assertion.
//   - A `?raw` import of a source file, or an import.meta.glob of source with a raw query, is a
//     source read.
import { readdirSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { parse } from "@babel/parser";

const SOURCE_EXTENSIONS = [".go", ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs"];
const READ_FUNCTIONS = new Set(["readFileSync", "readFile"]);
const TEMP_FUNCTIONS = new Set(["mkdtempSync", "mkdtemp", "tmpdir"]);
const PARSER_MODULES = new Set(["@babel/parser", "oxc-parser", "typescript", "acorn"]);
const ITERATING_METHODS = new Set(["map", "flatMap", "forEach", "filter", "some", "every", "find", "reduce"]);
const SOURCE_PATTERN = /\\\.\(?(?:\?:)?(?:go|[mc]?[jt]sx?)\b/;

// Path kinds, ordered: a higher kind wins when parts of a path combine.
const UNKNOWN = 0;
const ARTEFACT = 1;
const SOURCE = 2;
const GENERATED = 3;

const TEST_FILE = /\.(?:test|spec)\.(?:[mc]?[jt]sx?)$/;
const SKIPPED_DIRS = new Set([
  "node_modules",
  "dist",
  "storybook-static",
  "generated",
  "test-results",
  "playwright-report",
]);

// webTestFiles lists every test file under root, root-relative and slash-separated.
const webTestFiles = (root) => {
  const out = [];
  const walk = (dir) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      if (entry.name.startsWith(".") || SKIPPED_DIRS.has(entry.name)) continue;
      const path = join(dir, entry.name);
      if (entry.isDirectory()) walk(path);
      else if (TEST_FILE.test(entry.name)) out.push(relative(root, path).split(sep).join("/"));
    }
  };
  walk(root);
  return out.sort();
};

const literalKind = (text) => {
  if (text.split(/[/\\]/).includes("testdata")) return GENERATED;
  const bare = text.replace(/\?.*$/, "");
  if (SOURCE_EXTENSIONS.some((ext) => bare.endsWith(ext))) return SOURCE;
  return /\.[A-Za-z0-9]+$/.test(bare) && !/\s/.test(bare) ? ARTEFACT : UNKNOWN;
};

const isNode = (value) => value !== null && typeof value === "object" && typeof value.type === "string";

// walk visits every node with its ancestors, outermost first.
const walk = (node, visit, ancestors = []) => {
  visit(node, ancestors);
  const next = [...ancestors, node];
  for (const [key, value] of Object.entries(node)) {
    if (key === "loc" || key.endsWith("Comments") || key === "extra") continue;
    if (Array.isArray(value)) {
      for (const child of value) if (isNode(child)) walk(child, visit, next);
    } else if (isNode(value)) walk(value, visit, next);
  }
};

const calleeName = (callee) => {
  if (callee.type === "Identifier") return callee.name;
  if (callee.type === "MemberExpression" && !callee.computed && callee.property.type === "Identifier") {
    return callee.property.name;
  }
  return null;
};

const isFunction = (node) =>
  node.type === "ArrowFunctionExpression" ||
  node.type === "FunctionExpression" ||
  node.type === "FunctionDeclaration";

const patternNames = (pattern) => {
  const names = [];
  walk(pattern, (node, ancestors) => {
    const parent = ancestors.at(-1);
    if (node.type !== "Identifier") return;
    if (parent?.type === "ObjectProperty" && parent.key === node && !parent.shorthand) return;
    if (parent?.type === "TSTypeAnnotation" || ancestors.some((a) => a.type === "TSTypeAnnotation")) return;
    names.push(node.name);
  });
  return names;
};

// sourceTextReads returns the places a test file reads Go or TypeScript source as text, as
// { line, reason }.
const sourceTextReads = (source, file) => {
  const plugins = /\.[mc]?tsx?$/.test(file) ? ["typescript"] : [];
  if (/x$/.test(file) || !/\.[mc]?ts$/.test(file)) plugins.push("jsx");
  const program = parse(source, { sourceType: "unambiguous", plugins, errorRecovery: true }).program;

  const bindings = new Map(); // name -> init expressions
  const functionsByName = new Map(); // name -> function nodes
  const params = new Map(); // name -> [{ fn, index, owner }]
  const callsByName = new Map(); // name -> call nodes
  const parserNames = new Set(); // local names of parser functions/namespaces
  const parserInputs = new Set(); // names passed to a parser
  let selects = false;
  const found = [];
  const bind = (name, init) => {
    if (!bindings.has(name)) bindings.set(name, []);
    bindings.get(name).push(init);
  };

  walk(program, (node, ancestors) => {
    const parent = ancestors.at(-1);
    switch (node.type) {
      case "ImportDeclaration": {
        const spec = node.source.value;
        if (PARSER_MODULES.has(spec)) for (const s of node.specifiers) parserNames.add(s.local.name);
        const [path, query] = spec.split("?");
        if (query !== undefined && /(^|&)raw\b/.test(query) && literalKind(path) === SOURCE) {
          found.push({ line: node.loc.start.line, reason: `imports source ${spec} as text` });
        }
        break;
      }
      case "VariableDeclarator":
        if (node.init) for (const name of patternNames(node.id)) bind(name, node.init);
        if (node.id.type === "Identifier" && node.init && isFunction(node.init)) {
          functionsByName.set(node.id.name, [...(functionsByName.get(node.id.name) ?? []), node.init]);
        }
        break;
      case "AssignmentExpression":
        if (node.left.type === "Identifier") bind(node.left.name, node.right);
        break;
      case "ForOfStatement":
      case "ForInStatement": {
        const left = node.left.type === "VariableDeclaration" ? node.left.declarations[0].id : node.left;
        for (const name of patternNames(left)) bind(name, node.right);
        break;
      }
      case "FunctionDeclaration":
        if (node.id) functionsByName.set(node.id.name, [...(functionsByName.get(node.id.name) ?? []), node]);
        break;
      case "CallExpression": {
        const name = calleeName(node.callee);
        if (node.callee.type === "Identifier") {
          callsByName.set(name, [...(callsByName.get(name) ?? []), node]);
        }
        if (
          name === "endsWith" &&
          node.arguments.some((a) => a.type === "StringLiteral" && literalKind(a.value) === SOURCE)
        ) {
          selects = true;
        }
        if (
          name === "test" &&
          node.callee.type === "MemberExpression" &&
          node.callee.object.type === "RegExpLiteral" &&
          SOURCE_PATTERN.test(node.callee.object.pattern)
        ) {
          selects = true;
        }
        // import.meta.glob("./**/*.tsx", { query: "?raw" }) or { as: "raw" }.
        const callee = node.callee;
        if (
          callee.type === "MemberExpression" &&
          callee.object.type === "MetaProperty" &&
          calleeName(callee) === "glob" &&
          node.arguments[1]?.type === "ObjectExpression" &&
          node.arguments[1].properties.some(
            (p) => p.value?.type === "StringLiteral" && /^\??raw$/.test(p.value.value.replace(/^\?/, "")),
          ) &&
          globNamesSource(node.arguments[0])
        ) {
          found.push({ line: node.loc.start.line, reason: "globs source as raw text" });
        }
        break;
      }
      case "BinaryExpression":
        if (
          (node.operator === "===" || node.operator === "!==" || node.operator === "==") &&
          [node.left, node.right].some(
            (side) => side.type === "StringLiteral" && literalKind(side.value) === SOURCE,
          ) &&
          [node.left, node.right].some(
            (side) => side.type === "CallExpression" && calleeName(side.callee) === "extname",
          )
        ) {
          selects = true;
        }
        break;
      default:
        break;
    }
    if (isFunction(node)) {
      node.params.forEach((param, index) => {
        for (const name of patternNames(param)) {
          params.set(name, [...(params.get(name) ?? []), { fn: node, index, owner: parent }]);
        }
      });
    }
  });

  // Names whose value goes to a parser call are syntax-tree input.
  walk(program, (node) => {
    if (node.type !== "CallExpression") return;
    const callee = node.callee;
    const root = callee.type === "MemberExpression" ? callee.object : callee;
    if (root.type !== "Identifier" || !parserNames.has(root.name)) return;
    for (const arg of node.arguments) {
      walk(arg, (n) => {
        if (n.type === "Identifier") parserInputs.add(n.name);
      });
    }
  });

  const kindOf = (node, depth = 0, seen = new Set()) => {
    if (!node || depth > 10) return UNKNOWN;
    switch (node.type) {
      case "StringLiteral":
        return literalKind(node.value);
      case "TemplateLiteral": {
        const text = node.quasis.map((q) => q.value.cooked ?? "").join("x");
        return Math.max(literalKind(text), ...node.expressions.map((e) => kindOf(e, depth + 1, seen)));
      }
      case "BinaryExpression":
      case "LogicalExpression":
        return Math.max(kindOf(node.left, depth + 1, seen), kindOf(node.right, depth + 1, seen));
      case "ConditionalExpression":
        return Math.max(kindOf(node.consequent, depth + 1, seen), kindOf(node.alternate, depth + 1, seen));
      case "AwaitExpression":
      case "TSAsExpression":
      case "TSNonNullExpression":
      case "TSSatisfiesExpression":
      case "ParenthesizedExpression":
        return kindOf(node.expression ?? node.argument, depth + 1, seen);
      case "MemberExpression":
        return kindOf(node.object, depth + 1, seen);
      case "ArrayExpression":
        return Math.max(UNKNOWN, ...node.elements.map((e) => kindOf(e, depth + 1, seen)));
      case "ObjectExpression":
        return Math.max(
          UNKNOWN,
          ...node.properties.map((p) => kindOf(p.value ?? p.argument, depth + 1, seen)),
        );
      case "SpreadElement":
        return kindOf(node.argument, depth + 1, seen);
      case "NewExpression":
      case "CallExpression": {
        if (TEMP_FUNCTIONS.has(calleeName(node.callee))) return GENERATED;
        const parts = node.arguments.map((a) => kindOf(a, depth + 1, seen));
        if (node.callee.type === "MemberExpression") parts.push(kindOf(node.callee.object, depth + 1, seen));
        return Math.max(UNKNOWN, ...parts);
      }
      case "Identifier":
        return identKind(node.name, depth, seen);
      default:
        return UNKNOWN;
    }
  };

  const identKind = (name, depth, seen) => {
    if (seen.has(name)) return UNKNOWN;
    const next = new Set(seen).add(name);
    const parts = (bindings.get(name) ?? []).map((init) => kindOf(init, depth + 1, next));
    for (const { fn, index, owner } of params.get(name) ?? []) {
      // A named function's parameter takes the arguments at its call sites.
      for (const [fname, fns] of functionsByName) {
        if (!fns.includes(fn)) continue;
        for (const call of callsByName.get(fname) ?? [])
          parts.push(kindOf(call.arguments[index], depth + 1, next));
      }
      // A callback's parameter takes what the call iterates: list.map(cb) → list; it.each(list)(title, cb) → list.
      if (owner?.type === "CallExpression" && owner.arguments.includes(fn)) {
        const callee = owner.callee;
        if (callee.type === "MemberExpression" && ITERATING_METHODS.has(calleeName(callee))) {
          parts.push(kindOf(callee.object, depth + 1, next));
        } else if (callee.type === "CallExpression") {
          parts.push(...callee.arguments.map((a) => kindOf(a, depth + 1, next)));
        }
      }
    }
    return Math.max(UNKNOWN, ...parts);
  };

  walk(program, (node, ancestors) => {
    if (node.type !== "CallExpression" || !READ_FUNCTIONS.has(calleeName(node.callee))) return;
    const declarator = ancestors.findLast((a) => a.type === "VariableDeclarator");
    if (declarator && patternNames(declarator.id).some((n) => parserInputs.has(n))) return;
    const kind = kindOf(node.arguments[0]);
    if (kind === SOURCE) {
      found.push({ line: node.loc.start.line, reason: "reads a source file as text" });
    } else if (kind === UNKNOWN && selects) {
      found.push({
        line: node.loc.start.line,
        reason: "reads a file the test selected by a source extension",
      });
    }
  });

  return found.sort((a, b) => a.line - b.line);
};

const globNamesSource = (node) => {
  const patterns = node?.type === "ArrayExpression" ? node.elements : [node];
  return patterns.some(
    (p) => p?.type === "StringLiteral" && literalKind(p.value.replace(/\*+$/, "x.ts")) === SOURCE,
  );
};

export { sourceTextReads, webTestFiles };
