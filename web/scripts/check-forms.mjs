// The hand-rolled-form ledger (#2147 Wave 4). Every form is a TanStack Form (`useAppForm` in
// apps/web, `@tanstack/react-form` elsewhere): its values, validation and submit state live in one
// place rather than in a `useState` per input.
//
// A source is a hand-rolled form when it keeps a typed input's draft in `useState`: a `useState`
// setter is called from an input's `onChangeText` or `onChange` (a select's `onValueChange` picks
// what to show, which is a view setting, not a draft), and
// the file does not import `@tanstack/react-form` (or `@/lib/app-form`). A search box or filter
// (a setter named `setQuery`, `setFilter`, …) is not a form. Tests and stories are not production
// code and are not counted. A settings draft buffered through `useSettingsDraft` is the one
// sanctioned buffer that is not a TanStack form (see SETTINGS_DRAFT_HOOKS below).
//
// The committed ledger (hand-rolled-forms.json) is a ratchet:
//   - the check fails when a file not in the ledger is a hand-rolled form;
//   - it passes when forms are converted, and `--write` then drops them from the ledger.
// `--write` only ever drops files. It never adds one, so regenerating cannot accept a new
// hand-rolled form; that takes a hand edit to the JSON, visible in review. With no ledger yet,
// `--write` records the current state.
import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { parse } from "@babel/parser";

const webRoot = fileURLToPath(new URL("../", import.meta.url));
const roots = [join(webRoot, "apps", "web", "src"), join(webRoot, "packages", "ui", "src")];
const ledgerPath = join(webRoot, "hand-rolled-forms.json");

const INPUT_HANDLERS = new Set(["onChangeText", "onChange"]);
const FORM_MODULES = ["@tanstack/react-form", "@/lib/app-form"];
const NON_CHILD_KEYS = new Set([
  "loc",
  "start",
  "end",
  "extra",
  "leadingComments",
  "trailingComments",
  "innerComments",
]);

const isToolingFile = (path) =>
  path.includes(`${sep}test${sep}`) ||
  path.includes(`${sep}tests${sep}`) ||
  /\.(?:test|stories)\.(?:ts|tsx)$/.test(path) ||
  path.endsWith("routeTree.gen.ts");

const productionFiles = (directory) =>
  readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return productionFiles(path);
    return /\.tsx$/.test(entry.name) && !isToolingFile(path) ? [path] : [];
  });

// walk calls `visit` on every node under `node`.
const walk = (node, visit) => {
  if (!node || typeof node !== "object") return;
  if (Array.isArray(node)) {
    for (const child of node) walk(child, visit);
    return;
  }
  if (typeof node.type === "string") visit(node);
  for (const [key, child] of Object.entries(node)) {
    if (!NON_CHILD_KEYS.has(key) && child && typeof child === "object") walk(child, visit);
  }
};

// A search box or a list filter narrows what is shown; nothing submits it, so it is not a form.
const FILTER_SETTER = /^set(?:Query|Search|Filter|Q|Needle|Role|Status)\w*$/;

// stateSetters names every setter a `const [value, setValue] = useState(…)` declares, except a
// filter's.
const stateSetters = (program) => {
  const setters = new Set();
  walk(program, (node) => {
    if (
      node.type === "VariableDeclarator" &&
      node.id.type === "ArrayPattern" &&
      node.id.elements[1]?.type === "Identifier" &&
      node.init?.type === "CallExpression" &&
      node.init.callee.type === "Identifier" &&
      node.init.callee.name === "useState"
    ) {
      const setter = node.id.elements[1].name;
      if (!FILTER_SETTER.test(setter)) setters.add(setter);
    }
  });
  return setters;
};

const usesFormLibrary = (program) =>
  program.body.some((node) => node.type === "ImportDeclaration" && FORM_MODULES.includes(node.source.value));

// The settings draft is the one sanctioned buffer that is not a TanStack form. Settings and the
// setup wizard draw their fields from the settings registry (`SettingsFields`), so a field is a
// registry row, not a named form field; what they share is the buffer of changed keys. That buffer
// is `useSettingsDraft` (and the Settings layout's `useSettingsEdits`, built on it), imported from
// the settings-draft or settings-edits module. A named rule, not a side effect: before it, such a
// buffer passed only because its setter did not come from a `useState` in the file.
//
// The rule is two-sided. A settings-edit setter (`setEdit`, `setEdits`) destructured from the
// sanctioned hook is the sanctioned buffer. One destructured from any other hook is a draft kept
// somewhere this check cannot see into, so it counts like a `useState` setter and fails when an
// input writes it. A `useState` in the file is judged as before; the rule excuses nothing else.
const SETTINGS_DRAFT_HOOKS = new Set(["useSettingsDraft", "useSettingsEdits"]);
const SETTINGS_DRAFT_MODULE = /(?:^|\/)settings-(?:draft|edits)$/;
const DRAFT_SETTER = /^set(?:Edit|Edits)$/;

// sanctionedHookNames: the local names of the settings-draft hooks the file imports from their module.
const sanctionedHookNames = (program) => {
  const names = new Set();
  for (const node of program.body) {
    if (node.type !== "ImportDeclaration" || !SETTINGS_DRAFT_MODULE.test(node.source.value)) continue;
    for (const specifier of node.specifiers) {
      if (specifier.type === "ImportSpecifier" && SETTINGS_DRAFT_HOOKS.has(specifier.imported.name)) {
        names.add(specifier.local.name);
      }
    }
  }
  return names;
};

// draftHookCalls lists every `const { …, setEdit } = useSomething()`: the hook's name and the local
// names its draft setters were bound to.
const draftHookCalls = (program) => {
  const calls = [];
  walk(program, (node) => {
    if (
      node.type !== "VariableDeclarator" ||
      node.id.type !== "ObjectPattern" ||
      node.init?.type !== "CallExpression" ||
      node.init.callee.type !== "Identifier" ||
      !node.init.callee.name.startsWith("use") ||
      node.init.callee.name === "useState"
    ) {
      return;
    }
    const setters = node.id.properties
      .filter(
        (property) =>
          property.type === "ObjectProperty" &&
          property.key.type === "Identifier" &&
          DRAFT_SETTER.test(property.key.name) &&
          property.value.type === "Identifier",
      )
      .map((property) => property.value.name);
    if (setters.length > 0) calls.push({ hook: node.init.callee.name, setters });
  });
  return calls;
};

// usesSettingsDraft: the file buffers a settings draft through the sanctioned hook.
const usesSettingsDraft = (source) => {
  const program = parse(source, { sourceType: "module", plugins: ["typescript", "jsx"] }).program;
  const sanctioned = sanctionedHookNames(program);
  const calls = draftHookCalls(program);
  if (calls.some((call) => sanctioned.has(call.hook))) return true;
  // A provider (settings-edits) calls the hook for its edits without naming a setter.
  let called = false;
  walk(program, (node) => {
    if (
      node.type === "CallExpression" &&
      node.callee.type === "Identifier" &&
      sanctioned.has(node.callee.name)
    ) {
      called = true;
    }
  });
  return called;
};

// unsanctionedDraftSetters: settings-edit setters bound from a hook that is not the sanctioned one.
const unsanctionedDraftSetters = (program) => {
  const sanctioned = sanctionedHookNames(program);
  return draftHookCalls(program)
    .filter((call) => !sanctioned.has(call.hook))
    .flatMap((call) => call.setters);
};

// isHandRolledForm: a state setter is reachable from an input's change handler.
const isHandRolledForm = (source) => {
  const program = parse(source, { sourceType: "module", plugins: ["typescript", "jsx"] }).program;
  if (usesFormLibrary(program)) return false;
  const setters = stateSetters(program);
  for (const setter of unsanctionedDraftSetters(program)) setters.add(setter);
  if (setters.size === 0) return false;
  let found = false;
  walk(program, (node) => {
    if (found || node.type !== "JSXAttribute" || !INPUT_HANDLERS.has(node.name.name)) return;
    walk(node.value, (inner) => {
      if (inner.type === "Identifier" && setters.has(inner.name)) found = true;
    });
  });
  return found;
};

// measure returns the sorted paths (relative to web/) of every hand-rolled form.
const measure = (directories = roots) =>
  directories
    .flatMap((directory) => productionFiles(directory))
    .filter((file) => isHandRolledForm(readFileSync(file, "utf8")))
    .map((file) => relative(webRoot, file).split(sep).join("/"))
    .sort();

// settingsDraftUsers returns the sorted paths (relative to web/) of every file that buffers a
// settings draft through the sanctioned hook.
const settingsDraftUsers = (directories = roots) =>
  directories
    .flatMap((directory) => productionFiles(directory))
    .filter((file) => usesSettingsDraft(readFileSync(file, "utf8")))
    .map((file) => relative(webRoot, file).split(sep).join("/"))
    .sort();

// compareToLedger lists the forms the ledger does not allow and the ledger entries that could go.
const compareToLedger = (current, ledger) => ({
  canShrink: ledger.some((file) => !current.includes(file)),
  violations: current.filter((file) => !ledger.includes(file)),
});

// shrinkLedger drops the ledger entries that are no longer hand-rolled forms. It never adds one.
const shrinkLedger = (current, ledger) => ledger.filter((file) => current.includes(file));

const LEDGER_NOTE =
  "Hand-rolled forms (#2147 Wave 4), maintained by web/scripts/check-forms.mjs. Entries may only be removed: convert the form to useAppForm and run `pnpm forms:update`. Never add one to pass the check.";

const isMain = process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1];
if (isMain) {
  const current = measure();
  const hasLedger = existsSync(ledgerPath);
  const ledger = hasLedger ? JSON.parse(readFileSync(ledgerPath, "utf8")).files : [];
  if (process.argv.includes("--write")) {
    const files = hasLedger ? shrinkLedger(current, ledger) : current;
    writeFileSync(ledgerPath, `${JSON.stringify({ note: LEDGER_NOTE, files }, null, 2)}\n`);
    console.log(`forms: ledger ${hasLedger ? "shrunk" : "recorded"} — ${files.length} hand-rolled forms`);
  } else {
    const { violations, canShrink } = compareToLedger(current, ledger);
    if (violations.length > 0) {
      console.error("New hand-rolled forms (#2147): keep the draft in useAppForm, not useState:");
      for (const file of violations) console.error(`  ${file}`);
      process.exitCode = 1;
    } else {
      console.log(`forms: within the ledger — ${current.length} hand-rolled forms`);
      if (canShrink)
        console.log("forms: forms were converted; run `pnpm forms:update` to shrink the ledger.");
    }
  }
}

export { compareToLedger, isHandRolledForm, measure, settingsDraftUsers, shrinkLedger, usesSettingsDraft };
