// The hand-rolled-form ledger (#2147 Wave 4). Every form is a TanStack Form (`useAppForm` in
// apps/web, `@tanstack/react-form` elsewhere): its values, validation and submit state live in one
// place rather than in a `useState` per input.
//
// A source is a hand-rolled form when it keeps a typed input's draft in `useState`: a `useState`
// setter is called from an input's `onChangeText` or `onChange` (a select's `onValueChange` picks
// what to show, which is a view setting, not a draft). A handler counts when it names a function of
// the same file that stores what it was handed (`onChangeText={changeNote}` with a `changeNote` that
// calls `setNote(value)`), and so does a text state's setter handed whole to a change prop
// (`onDsnChange={setDsn}`). Each input is judged on its own: a file that
// also holds a TanStack form is not excused, since a field's `handleChange` is never a `useState`
// setter and so never counts. A search box or filter
// (a setter named `setQuery`, `setSearch`, `setFilter`, …) is not a form, and neither is a flag a
// handler raises (`setOpen(true)`). Tests and stories are not production code and are not counted.
// Two buffers are sanctioned that are not a TanStack form: a settings draft (`useSettingsDraft`) and
// a single input's draft-until-commit (`useCommittedDraft`); see SANCTIONED_DRAFT_HOOKS below.
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

// Two buffers are sanctioned that are not a TanStack form. Each is a named rule, not a side effect:
// before them, such a buffer passed only because its setter did not come from a `useState` in the
// file.
//
//   settings — Settings and the setup wizard draw their fields from the settings registry
//   (`SettingsFields`), so a field is a registry row, not a named form field; what they share is
//   the buffer of changed keys. That buffer is `useSettingsDraft` (and the Settings layout's
//   `useSettingsEdits`, built on it), imported from the settings-draft or settings-edits module.
//
//   committed — a single input that is not a form: a value edited beside the stored one and handed
//   over on blur or Enter (a box that saves on leaving it, a path-map row, a From/To pair). That
//   draft is `useCommittedDraft`, imported from the use-committed-draft module. Inputs submitted
//   together, with validation, are a form and use `useUiForm` / `useAppForm`.
//
// The rules are two-sided. A draft setter (`setEdit`, `setEdits`, `editDraft`) destructured from a
// sanctioned hook, imported from its own module, is the sanctioned buffer. One bound from any other
// hook (an object pattern, or the `[value, setValue]` pair a lookalike of `useState` returns) is a
// draft kept somewhere this check cannot see into, so it counts like a `useState` setter and fails
// when an input writes it. A `useState` in the file is judged as before; the rules excuse nothing
// else.
const SANCTIONED_DRAFT_HOOKS = [
  {
    group: "settings",
    hooks: new Set(["useSettingsDraft", "useSettingsEdits"]),
    module: /(?:^|\/)settings-(?:draft|edits)$/,
  },
  {
    group: "committed",
    hooks: new Set(["useCommittedDraft"]),
    // Inside the UI package it is imported from its own module; the app takes it from the package.
    module: /(?:^|\/)use-committed-draft$|^@loomarr\/ui$/,
  },
];
const DRAFT_SETTER = /^(?:set(?:Edit|Edits)|editDraft)$/;
const PAIR_SETTER = /^set[A-Z]\w*$/;

// sanctionedHookNames: the local names of the sanctioned draft hooks the file imports from their
// module. `groups` narrows it to some of them; by default every sanctioned hook counts.
const sanctionedHookNames = (program, groups = SANCTIONED_DRAFT_HOOKS.map((entry) => entry.group)) => {
  const names = new Set();
  for (const node of program.body) {
    if (node.type !== "ImportDeclaration") continue;
    for (const entry of SANCTIONED_DRAFT_HOOKS) {
      if (!groups.includes(entry.group) || !entry.module.test(node.source.value)) continue;
      for (const specifier of node.specifiers) {
        if (specifier.type === "ImportSpecifier" && entry.hooks.has(specifier.imported.name)) {
          names.add(specifier.local.name);
        }
      }
    }
  }
  return names;
};

const isHookCall = (init) =>
  init?.type === "CallExpression" &&
  init.callee.type === "Identifier" &&
  init.callee.name.startsWith("use") &&
  init.callee.name !== "useState";

// draftHookCalls lists every `const { …, setEdit } = useSomething()` and every
// `const [value, setValue] = useSomething()`: the hook's name and the local names its draft setters
// were bound to.
const draftHookCalls = (program) => {
  const calls = [];
  walk(program, (node) => {
    if (node.type !== "VariableDeclarator" || !isHookCall(node.init)) return;
    let setters = [];
    if (node.id.type === "ObjectPattern") {
      setters = node.id.properties
        .filter(
          (property) =>
            property.type === "ObjectProperty" &&
            property.key.type === "Identifier" &&
            DRAFT_SETTER.test(property.key.name) &&
            property.value.type === "Identifier",
        )
        .map((property) => property.value.name);
    } else if (
      node.id.type === "ArrayPattern" &&
      node.id.elements[1]?.type === "Identifier" &&
      PAIR_SETTER.test(node.id.elements[1].name) &&
      !FILTER_SETTER.test(node.id.elements[1].name)
    ) {
      setters = [node.id.elements[1].name];
    }
    if (setters.length > 0) calls.push({ hook: node.init.callee.name, setters });
  });
  return calls;
};

// usesDraftHook: the file buffers a draft through a sanctioned hook of this group.
const usesDraftHook = (source, group) => {
  const program = parse(source, { sourceType: "module", plugins: ["typescript", "jsx"] }).program;
  const sanctioned = sanctionedHookNames(program, [group]);
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

// usesSettingsDraft: the file buffers a settings draft through the sanctioned hook.
const usesSettingsDraft = (source) => usesDraftHook(source, "settings");

// usesCommittedDraft: the file keeps a single input's draft through the sanctioned hook.
const usesCommittedDraft = (source) => usesDraftHook(source, "committed");

// unsanctionedDraftSetters: draft setters bound from a hook that is not a sanctioned one.
const unsanctionedDraftSetters = (program) => {
  const sanctioned = sanctionedHookNames(program);
  return draftHookCalls(program)
    .filter((call) => !sanctioned.has(call.hook))
    .flatMap((call) => call.setters);
};

// localFunctions maps the name of every function the file binds (`const f = () => …`,
// `const f = function () …`, `function f() …`) to the function, so a handler that names one is
// followed into it.
const localFunctions = (program) => {
  const functions = new Map();
  walk(program, (node) => {
    if (node.type === "FunctionDeclaration" && node.id) functions.set(node.id.name, node);
    if (
      node.type === "VariableDeclarator" &&
      node.id.type === "Identifier" &&
      (node.init?.type === "ArrowFunctionExpression" || node.init?.type === "FunctionExpression")
    ) {
      functions.set(node.id.name, node.init);
    }
  });
  return functions;
};

// storesArgument: the function calls one of `setters` with something derived from its own
// parameters, which is how a wrapper stores what an input handed it. A wrapper that resets a page
// or a cursor beside navigating carries nothing the viewer typed.
const storesArgument = (fn, setters) => {
  const parameters = new Set();
  for (const parameter of fn.params) {
    walk(parameter, (inner) => {
      if (inner.type === "Identifier") parameters.add(inner.name);
    });
  }
  let stores = false;
  walk(fn.body, (inner) => {
    if (inner.type !== "CallExpression" || inner.callee.type !== "Identifier") return;
    if (!setters.has(inner.callee.name)) return;
    for (const argument of inner.arguments) {
      walk(argument, (name) => {
        if (name.type === "Identifier" && parameters.has(name.name)) stores = true;
      });
    }
  });
  return stores;
};

// A change prop is where a component hands back what the viewer typed. A select's `onValueChange`,
// a dialog's `onOpenChange` and a switch's `onCheckedChange` pick a view or raise a flag.
const CHANGE_PROP = /^on[A-Z]\w*Change(?:Text)?$/;
const VIEW_CHANGE_PROPS = new Set(["onValueChange", "onOpenChange", "onCheckedChange"]);

// textSetters names the setters of `useState` calls that hold text: a string, a template or a
// `useState<string>`. A bare setter handed to a change prop is a draft only when it holds text; a
// stepper's position or a chosen view is client state, not something typed.
const textSetters = (program) => {
  const setters = new Set();
  walk(program, (node) => {
    if (
      node.type !== "VariableDeclarator" ||
      node.id.type !== "ArrayPattern" ||
      node.id.elements[1]?.type !== "Identifier" ||
      node.init?.type !== "CallExpression" ||
      node.init.callee.type !== "Identifier" ||
      node.init.callee.name !== "useState"
    ) {
      return;
    }
    const initial = node.init.arguments[0];
    const declared = node.init.typeArguments?.params?.[0];
    if (
      initial?.type === "StringLiteral" ||
      initial?.type === "TemplateLiteral" ||
      declared?.type === "TSStringKeyword"
    ) {
      setters.add(node.id.elements[1].name);
    }
  });
  return setters;
};

// isHandRolledForm: a state setter is reachable from an input's change handler, whether the
// handler is the setter, calls it, or names a function in this file that does; or a text state's
// setter is handed to a change prop for another component to write from its own input.
const isHandRolledForm = (source) => {
  const program = parse(source, { sourceType: "module", plugins: ["typescript", "jsx"] }).program;
  const setters = stateSetters(program);
  for (const setter of unsanctionedDraftSetters(program)) setters.add(setter);
  if (setters.size === 0) return false;
  const functions = localFunctions(program);
  const text = textSetters(program);

  // storesThrough: a function an input's handler names stores what it is handed, directly or
  // through another function in the file.
  const storesThrough = (fn, seen) => {
    if (storesArgument(fn, setters)) return true;
    let stores = false;
    walk(fn.body, (inner) => {
      if (stores || inner.type !== "Identifier" || !functions.has(inner.name) || seen.has(inner.name)) return;
      seen.add(inner.name);
      stores = storesThrough(functions.get(inner.name), seen);
    });
    return stores;
  };

  // writesSetter: `root` writes a setter, looking into the local functions it names. `setOpen(true)`
  // raises a flag and `setError(undefined)` clears what the last submit said; neither carries
  // anything the viewer typed. An `onChange` prop on a component that is not an input (an editor
  // that reports a changed list) often does the first.
  const writesSetter = (root, follow, seen = new Set()) => {
    const flags = new Set();
    walk(root, (inner) => {
      const argument = inner.type === "CallExpression" ? inner.arguments[0] : undefined;
      if (
        inner.type === "CallExpression" &&
        inner.callee.type === "Identifier" &&
        inner.arguments.length === 1 &&
        (argument.type === "BooleanLiteral" ||
          argument.type === "NullLiteral" ||
          (argument.type === "Identifier" && argument.name === "undefined"))
      ) {
        flags.add(inner.callee);
      }
    });
    let writes = false;
    walk(root, (inner) => {
      if (writes || inner.type !== "Identifier" || flags.has(inner)) return;
      if (setters.has(inner.name)) {
        writes = true;
      } else if (follow && functions.has(inner.name) && !seen.has(inner.name)) {
        seen.add(inner.name);
        writes = storesThrough(functions.get(inner.name), seen);
      }
    });
    return writes;
  };

  let found = false;
  walk(program, (element) => {
    if (found || element.type !== "JSXOpeningElement") return;
    // A handler is followed into a function of the file when it is an input's: a field's
    // `onChangeText`, or a DOM input's `onChange`. A component's `onChange` reports a structured
    // value (an editor's changed list), so only a setter it names outright counts.
    const intrinsic = element.name.type === "JSXIdentifier" && /^[a-z]/.test(element.name.name);
    for (const node of element.attributes) {
      if (node.type !== "JSXAttribute" || node.name.type !== "JSXIdentifier") continue;
      const name = node.name.name;
      if (INPUT_HANDLERS.has(name)) {
        found ||= writesSetter(node.value, name === "onChangeText" || intrinsic);
      } else if (
        CHANGE_PROP.test(name) &&
        !VIEW_CHANGE_PROPS.has(name) &&
        node.value?.type === "JSXExpressionContainer" &&
        node.value.expression.type === "Identifier"
      ) {
        found ||= text.has(node.value.expression.name) && setters.has(node.value.expression.name);
      }
    }
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

// committedDraftUsers returns the sorted paths (relative to web/) of every file that keeps a single
// input's draft through the sanctioned hook.
const committedDraftUsers = (directories = roots) =>
  directories
    .flatMap((directory) => productionFiles(directory))
    .filter((file) => usesCommittedDraft(readFileSync(file, "utf8")))
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

export {
  committedDraftUsers,
  compareToLedger,
  isHandRolledForm,
  measure,
  settingsDraftUsers,
  shrinkLedger,
  usesCommittedDraft,
  usesSettingsDraft,
};
