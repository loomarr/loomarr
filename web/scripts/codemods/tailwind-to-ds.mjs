// Moves the common, unambiguous Tailwind layout and type utilities onto the design system's `Stack`
// and `Text` props (#2147). It is a text-splice codemod over a Babel AST: every change replaces the
// exact source range of a tag name or a className attribute, so comments, formatting and everything
// else in the file are untouched. Run `biome check --write` over the changed files afterwards.
//
//   node scripts/codemods/tailwind-to-ds.mjs [--write] [--blocks] <file or glob>...
//
// Without --write it prints the report and changes nothing. It is idempotent: a second run finds
// nothing left to move. `--blocks` also converts a plain block div whose children are all block
// elements (a judgement call, so it is opt-in: review the pictures).
//
// What it moves (only from a className that is one plain string, never cn(...) or a template with
// expressions):
//   <div|span|section|…> with `flex`      → <Stack>: direction, wrap, align, justify, gap (and the
//                                            sm/md/lg forms of those four), padding, margin, width,
//                                            height, maxWidth, minWidth, minHeight, flexGrow/Shrink/Basis
//   <Stack className="…">                 → the same props, except the layout ones the Stack's own base
//                                            rule already decides (see below)
//   <p|span|h1…h6|dt|dd> with a size      → <Text as="…" textRole tone strong shout>
//   <Text textRole className="text-sm">   → drops the size, weight, leading, tracking and colour
//                                            classes the role's inline style already overrides
//
// What it never touches: a class with a variant it cannot express (hover:, focus-visible:, group-*,
// dark:, @md:, an unlisted breakpoint), an arbitrary value, `grid`, a class it does not know, a
// dynamic className, an element with an event handler or a spread. Leaving a class is always safe.
//
// Traps it encodes (docs/frontend-design.md, the Stack notes in the project memory):
//   - A Stack's base rule is unlayered and beats Tailwind: display, flex-direction, align-items,
//     justify-content, flex-wrap, gap, min-width, min-height, flex-basis and flex-shrink. Every one
//     of those classes on an element must therefore become a prop, or the element is not converted.
//   - A Stack is a column; a former `flex` row needs direction="row".
//   - A Stack's default shrink follows the Stack above it (#4059: 0 in a column, 1 in a row), where a
//     div always shrank. So the codemod never relies on the default: a converted element that shrank
//     (flex-1, shrink, min-w-0, min-h-0, overflow) takes an explicit flexShrink={1}, shrink-0 an
//     explicit flexShrink={0}, and `flex-1` is flexBasis="0%" flexGrow={1} flexShrink={1}.
//   - `Text as="…"` renders a plain element, so no layout prop is ever put on a Text.
//   - `grid` is not a Stack, so an element with it is left alone.
//   - A Stack that already has a class for a hard property keeps it: the class is dead today, and
//     turning it into a prop would change the picture.
import { globSync, readFileSync, writeFileSync } from "node:fs";
import { relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parse } from "@babel/parser";

const DESIGN_SYSTEM = "@loomarr/design-system";

// Elements that become a Stack, with the `render` the Stack needs to stay that element.
const STACK_TAGS = {
  article: "article",
  aside: "aside",
  div: null,
  footer: "footer",
  form: "form",
  header: "header",
  li: "li",
  main: "main",
  nav: "nav",
  ol: "ol",
  section: "section",
  span: "span",
  ul: "ul",
};
// Attributes a converted element may carry: the Stack passes them to its View unchanged.
const FORM_ATTRIBUTES = new Set(["action", "method", "noValidate", "onSubmit"]);
// `ref` is not here: a Stack's ref is typed as a Tamagui element, not the DOM element a div's is.
const PLAIN_ATTRIBUTES = new Set(["className", "id", "key", "role", "style"]);
const TEXT_TAGS = new Set(["p", "span", "h1", "h2", "h3", "h4", "h5", "h6", "dt", "dd"]);

const SPACE_STEPS = new Set(["0.5", "1", "1.5", "1.75", "2", "2.5", "3", "4", "5", "6", "7", "8"]);
const ALIGN = { baseline: "baseline", center: "center", end: "end", start: "start", stretch: "stretch" };
const JUSTIFY = { between: "between", center: "center", end: "end", start: "start" };
const BREAKPOINTS = ["sm", "md", "lg"];

const splitVariants = (raw) => {
  const parts = [];
  let depth = 0;
  let current = "";
  for (const char of raw) {
    if (char === "[") depth += 1;
    if (char === "]") depth -= 1;
    if (char === ":" && depth === 0) {
      parts.push(current);
      current = "";
    } else current += char;
  }
  parts.push(current);
  return parts;
};

const parseToken = (raw) => {
  const parts = splitVariants(raw);
  let util = parts.pop();
  let important = false;
  if (util.startsWith("!")) {
    important = true;
    util = util.slice(1);
  }
  if (util.endsWith("!")) {
    important = true;
    util = util.slice(0, -1);
  }
  let negative = false;
  if (util.startsWith("-")) {
    negative = true;
    util = util.slice(1);
  }
  return { important, negative, raw, util, variants: parts };
};

// A Tailwind spacing key as pixels: `4` is 16, `1.5` is 6. Null when it is not a whole pixel count.
const spacePixels = (key) => {
  if (!/^\d+(?:\.\d+)?$/.test(key)) return null;
  const pixels = Number(key) * 4;
  return Number.isInteger(pixels) ? pixels : null;
};

const SIDE_PROPS = { b: "Bottom", l: "Left", r: "Right", t: "Top" };

// The family a token belongs to. A family is converted whole or left whole, so a leftover class never
// contradicts a prop that came from its siblings.
const familyOf = ({ util }) => {
  if (
    /^(?:flex|inline-flex|block|inline-block|inline|hidden|grid|inline-grid|contents|table(?:-.+)?|flow-root|list-item)$/.test(
      util,
    )
  )
    return "display";
  if (/^flex-(?:row|col)(?:-reverse)?$/.test(util)) return "direction";
  if (/^flex-(?:wrap|nowrap|wrap-reverse)$/.test(util)) return "wrap";
  if (/^items-/.test(util)) return "align";
  if (/^justify-(?!items|self)/.test(util)) return "justify";
  if (/^gap$|^gap-/.test(util)) return "gap";
  if (/^[pm][xytrbl]?-/.test(util)) return util[0] === "p" ? "padding" : "margin";
  if (/^w-/.test(util)) return "width";
  if (/^h-/.test(util)) return "height";
  if (/^max-w-/.test(util)) return "maxWidth";
  if (/^min-w-/.test(util)) return "minWidth";
  if (/^min-h-/.test(util)) return "minHeight";
  if (
    /^(?:flex-(?:1|auto|none|initial|\[.*\])|grow(?:-.*)?|shrink(?:-.*)?|flex-grow(?:-.*)?|flex-shrink(?:-.*)?|basis-.*)$/.test(
      util,
    )
  )
    return "flexItem";
  return null;
};

// The families whose property the Stack's own rules always set, so a class for one never wins.
const HARD_FAMILIES = [
  "display",
  "direction",
  "wrap",
  "align",
  "justify",
  "gap",
  "minWidth",
  "minHeight",
  "flexItem",
];

// responsive collects one property's tokens into { base, sm, md, lg } and returns null when a token
// has a variant the Stack cannot express, appears twice at one width, or maps to nothing.
const responsive = (tokens, mapUtil) => {
  const found = {};
  for (const token of tokens) {
    if (token.important || token.negative) return null;
    if (token.variants.length > 1) return null;
    const width = token.variants.length === 0 ? "base" : token.variants[0];
    if (width !== "base" && !BREAKPOINTS.includes(width)) return null;
    const value = mapUtil(token.util);
    if (value === undefined || value === null || width in found) return null;
    found[width] = value;
  }
  return found;
};

// attributeText renders a `{ base, sm }` map as a JSX attribute value for `name`, or "" when the map
// holds only the Stack's own default.
const responsiveAttribute = (name, found, baseDefault, stackDefault) => {
  const widths = ["base", ...BREAKPOINTS].filter((width) => width in found || width === "base");
  const withBase = { ...found, base: found.base ?? baseDefault };
  if (widths.length === 1) {
    return withBase.base === stackDefault ? null : `${name}=${JSON.stringify(withBase.base)}`;
  }
  const body = widths.map((width) => `${width}: ${JSON.stringify(withBase[width])}`).join(", ");
  return `${name}={{ ${body} }}`;
};

const gapKey = (util, prefix) => {
  const key = util.slice(prefix.length);
  if (key === "0") return "none";
  return SPACE_STEPS.has(key) ? key : null;
};

// Each family converter returns { props: [[name, attributeText]], consumed: [raw…] } or null when the
// family has a token it cannot express.
const convertDirection = (tokens, { baseDirection }) => {
  const found = responsive(tokens, (util) =>
    util === "flex-row" ? "row" : util === "flex-col" ? "column" : null,
  );
  if (!found) return null;
  const text = responsiveAttribute("direction", found, baseDirection, "column");
  return { consumed: tokens.map((token) => token.raw), props: text ? [["direction", text]] : [] };
};

const convertWrap = (tokens) => {
  const consumed = [];
  let wraps = false;
  for (const token of tokens) {
    if (token.variants.length > 0 || token.important || token.util === "flex-wrap-reverse") return null;
    wraps = token.util === "flex-wrap";
    consumed.push(token.raw);
  }
  return { consumed, props: wraps ? [["wrap", "wrap"]] : [] };
};

const convertAlign = (tokens) => {
  const found = responsive(tokens, (util) => ALIGN[util.slice("items-".length)]);
  if (!found) return null;
  const text = responsiveAttribute("align", found, "stretch", "stretch");
  return { consumed: tokens.map((token) => token.raw), props: text ? [["align", text]] : [] };
};

const convertJustify = (tokens) => {
  const found = responsive(tokens, (util) => JUSTIFY[util.slice("justify-".length)]);
  if (!found) return null;
  const text = responsiveAttribute("justify", found, "start", "start");
  return { consumed: tokens.map((token) => token.raw), props: text ? [["justify", text]] : [] };
};

const convertGap = (tokens) => {
  const byAxis = { columnGap: [], gap: [], rowGap: [] };
  for (const token of tokens) {
    if (token.util === "gap" || /^gap-[xy]$/.test(token.util)) return null;
    if (token.util.startsWith("gap-x-")) byAxis.columnGap.push(token);
    else if (token.util.startsWith("gap-y-")) byAxis.rowGap.push(token);
    else byAxis.gap.push(token);
  }
  const props = [];
  for (const [name, axis] of Object.entries(byAxis)) {
    if (axis.length === 0) continue;
    const prefix = name === "gap" ? "gap-" : name === "columnGap" ? "gap-x-" : "gap-y-";
    const found = responsive(axis, (util) => gapKey(util, prefix));
    // A per-axis gap overrides `gap` on its axis, so it needs a value at every width.
    if (!found || (name !== "gap" && !("base" in found))) return null;
    const text = responsiveAttribute(name, found, "none", name === "gap" ? "none" : "never");
    if (text) props.push([name, text]);
  }
  return { consumed: tokens.map((token) => token.raw), props };
};

// A side map from p-/px-/py-/pt-… tokens, applied in Tailwind's order (all, axis, side), then written
// as the fewest props that say the same.
const convertBox = (tokens, kind) => {
  const sides = {};
  const rank = (util) => (/^[pm]-/.test(util) ? 0 : /^[pm][xy]-/.test(util) ? 1 : 2);
  const sorted = [...tokens].sort((a, b) => rank(a.util) - rank(b.util));
  for (const token of sorted) {
    if (token.variants.length > 0 || token.important) return null;
    const match = /^[pm]([xytrbl]?)-(.+)$/.exec(token.util);
    if (!match) return null;
    const [, side, key] = match;
    let value;
    if (kind === "margin" && key === "auto") {
      if (token.negative) return null;
      value = "auto";
    } else {
      const pixels = spacePixels(key);
      if (pixels === null) return null;
      value = token.negative ? -pixels : pixels;
      if (kind === "padding" && token.negative) return null;
    }
    const targets =
      side === "" ? ["t", "r", "b", "l"] : side === "x" ? ["l", "r"] : side === "y" ? ["t", "b"] : [side];
    for (const target of targets) sides[target] = value;
  }
  const prefix = kind;
  const text = (value) => (typeof value === "string" ? JSON.stringify(value) : `{${value}}`);
  const same = (a, b) => a !== undefined && a === b;
  const props = [];
  if (["t", "r", "b", "l"].every((side) => same(sides.t, sides[side]))) {
    props.push([prefix, `${prefix}=${text(sides.t)}`]);
  } else {
    if (same(sides.t, sides.b)) props.push([`${prefix}Vertical`, `${prefix}Vertical=${text(sides.t)}`]);
    else
      for (const side of ["t", "b"])
        if (sides[side] !== undefined)
          props.push([`${prefix}${SIDE_PROPS[side]}`, `${prefix}${SIDE_PROPS[side]}=${text(sides[side])}`]);
    if (same(sides.l, sides.r)) props.push([`${prefix}Horizontal`, `${prefix}Horizontal=${text(sides.l)}`]);
    else
      for (const side of ["l", "r"])
        if (sides[side] !== undefined)
          props.push([`${prefix}${SIDE_PROPS[side]}`, `${prefix}${SIDE_PROPS[side]}=${text(sides[side])}`]);
  }
  return { consumed: tokens.map((token) => token.raw), props };
};

const SIZE_KEYWORDS = {
  height: { full: "100%", screen: "100vh" },
  maxWidth: { full: "100%" },
  minHeight: { full: "100%", screen: "100vh" },
  minWidth: { full: "100%" },
  width: { full: "100%", screen: "100vw" },
};

const convertSize = (tokens, family) => {
  const props = [];
  for (const token of tokens) {
    if (token.variants.length > 0 || token.important || token.negative) return null;
    const prefix = { height: "h-", maxWidth: "max-w-", minHeight: "min-h-", minWidth: "min-w-", width: "w-" }[
      family
    ];
    const key = token.util.slice(prefix.length);
    const keyword = SIZE_KEYWORDS[family][key];
    if (keyword !== undefined) props.push([family, `${family}=${JSON.stringify(keyword)}`]);
    else if ((family === "minWidth" || family === "minHeight") && spacePixels(key) !== null) {
      props.push([family, `${family}={${spacePixels(key)}}`]);
    } else return null;
  }
  if (new Set(props.map(([name]) => name)).size !== props.length) return null;
  return { consumed: tokens.map((token) => token.raw), props };
};

// flex-1 and its kin, resolved in Tailwind's order: the `flex` shorthand first, then grow, shrink and
// basis over it.
const convertFlexItem = (tokens) => {
  const state = { basis: undefined, grow: undefined, shrink: undefined };
  const isShorthand = (token) => /^flex-(?:1|auto|none|initial|\[.*\])$/.test(token.util);
  for (const token of [...tokens.filter(isShorthand), ...tokens.filter((item) => !isShorthand(item))]) {
    if (token.variants.length > 0 || token.important || token.negative) return null;
    const { util } = token;
    if (util === "flex-1") Object.assign(state, { basis: "0%", grow: 1, shrink: 1 });
    else if (util === "flex-auto") Object.assign(state, { basis: undefined, grow: 1, shrink: 1 });
    else if (util === "flex-none") Object.assign(state, { basis: undefined, grow: 0, shrink: 0 });
    else if (util === "flex-initial") Object.assign(state, { basis: undefined, grow: 0, shrink: 1 });
    else if (util === "grow" || util === "flex-grow") state.grow = 1;
    else if (util === "grow-0" || util === "flex-grow-0") state.grow = 0;
    else if (util === "shrink" || util === "flex-shrink") state.shrink = 1;
    else if (util === "shrink-0" || util === "flex-shrink-0") state.shrink = 0;
    else if (util === "basis-0") state.basis = 0;
    else if (util === "basis-full") state.basis = "100%";
    else if (util === "basis-auto") state.basis = undefined;
    else return null;
  }
  const props = [];
  if (state.basis !== undefined) {
    props.push([
      "flexBasis",
      typeof state.basis === "string"
        ? `flexBasis=${JSON.stringify(state.basis)}`
        : `flexBasis={${state.basis}}`,
    ]);
  }
  if (state.grow === 1) props.push(["flexGrow", "flexGrow={1}"]);
  if (state.shrink !== undefined) props.push(["flexShrink", `flexShrink={${state.shrink}}`]);
  return { consumed: tokens.map((token) => token.raw), props };
};

const CONVERTERS = {
  align: convertAlign,
  direction: convertDirection,
  flexItem: convertFlexItem,
  gap: convertGap,
  height: (tokens) => convertSize(tokens, "height"),
  justify: convertJustify,
  margin: (tokens) => convertBox(tokens, "margin"),
  maxWidth: (tokens) => convertSize(tokens, "maxWidth"),
  minHeight: (tokens) => convertSize(tokens, "minHeight"),
  minWidth: (tokens) => convertSize(tokens, "minWidth"),
  padding: (tokens) => convertBox(tokens, "padding"),
  width: (tokens) => convertSize(tokens, "width"),
  wrap: convertWrap,
};

// Classes on an existing Stack that name what its base rule already does: min-width and min-height 0.
// Moving them to props changes nothing on screen. `shrink-0` is not one of them: the Stack's own
// flexShrink beats it, so it is dead, and the prop would start to hold in a row.
const STACK_AGREES = new Set(["min-w-0", "min-h-0"]);

// layoutPlan decides what a className becomes on a Stack. `isNew` is a div being turned into one: it
// must be a plain `flex` container and every hard family must convert. An existing Stack converts only
// the families its base rule leaves to Tailwind.
const layoutPlan = (tokens, { existing, isNew, block = false }) => {
  const byFamily = {};
  for (const token of tokens) {
    const family = familyOf(token);
    if (family) byFamily[family] = [...(byFamily[family] ?? []), token];
  }
  const consumed = new Set();
  const props = new Map();
  const reasons = [];
  if (isNew && block) {
    // A block container has no display class to move and stays a column.
    if (byFamily.display) return { reason: "display class on a block container" };
  } else if (isNew) {
    const display = byFamily.display ?? [];
    const plain =
      display.length === 1 &&
      display[0].util === "flex" &&
      display[0].variants.length === 0 &&
      !display[0].important;
    if (!plain)
      return {
        reason:
          display.length === 0
            ? "not a flex container"
            : `display ${display.map((token) => token.raw).join(" ")}`,
      };
    consumed.add(display[0].raw);
  } else if (byFamily.display) {
    const flexOnly = byFamily.display.filter(
      (token) => token.util === "flex" && token.variants.length === 0 && !token.important,
    );
    for (const token of flexOnly) consumed.add(token.raw);
    if (flexOnly.length !== byFamily.display.length)
      reasons.push(`display ${byFamily.display.map((token) => token.raw).join(" ")}`);
  }
  const baseDirection = isNew && !block ? "row" : "column";
  for (const [family, allTokens] of Object.entries(byFamily)) {
    if (family === "display") continue;
    let familyTokens = allTokens;
    if (!isNew && HARD_FAMILIES.includes(family)) {
      // The Stack's base rule already decides these, so most classes are dead and turning one into a
      // prop would change the picture. These three say what the Stack does anyway.
      const same = allTokens.filter((token) => STACK_AGREES.has(token.raw));
      const dead = allTokens.filter((token) => !STACK_AGREES.has(token.raw));
      if (dead.length > 0)
        reasons.push(`${family} class on a Stack: ${dead.map((token) => token.raw).join(" ")}`);
      if (same.length === 0) continue;
      familyTokens = same;
    }
    const converted = CONVERTERS[family]?.(familyTokens, { baseDirection });
    if (!converted) {
      reasons.push(`${family}: ${familyTokens.map((token) => token.raw).join(" ")}`);
      if (isNew && HARD_FAMILIES.includes(family))
        return { reason: `unmapped ${family} class ${familyTokens.map((token) => token.raw).join(" ")}` };
      continue;
    }
    const clash = converted.props.find(([name]) => existing.has(name));
    if (clash) {
      reasons.push(`${family}: ${clash[0]} is already a prop`);
      if (isNew && HARD_FAMILIES.includes(family)) return { reason: `${clash[0]} is already a prop` };
      continue;
    }
    for (const [name, text] of converted.props) props.set(name, text);
    for (const raw of converted.consumed) consumed.add(raw);
  }
  if (isNew) {
    // A direction is explicit when the container was a row, even with no other layout class.
    if (!block && !props.has("direction") && !byFamily.direction) props.set("direction", 'direction="row"');
    if (block && consumed.size === 0) return { reason: "nothing a Stack would take" };
    const shrinks = tokens.some(
      (token) => token.variants.length === 0 && /^(?:min-w-0|min-h-0|overflow-.+|truncate)$/.test(token.util),
    );
    if (shrinks && !props.has("flexShrink") && !existing.has("flexShrink"))
      props.set("flexShrink", "flexShrink={1}");
  }
  return { consumed, props, reasons };
};

// ---- Text -----------------------------------------------------------------------------------------

const TEXT_ALIGN_OR_WRAP =
  /^(?:left|center|right|justify|start|end|wrap|nowrap|balance|pretty|ellipsis|clip)$/;
const TEXT_SIZES = new Set(["2xs", "xs", "sm", "base", "md", "lg", "xl", "2xl"]);
const TONES = {
  "text-caution": "warning",
  "text-destructive": "danger",
  "text-foreground": "primary",
  "text-lock": "success",
  "text-muted-foreground": "secondary",
  "text-onair": "danger",
  "text-signal": "signal",
  "text-static-100": "primary",
  "text-static-400": "secondary",
  "text-suggest": "suggest",
  "text-tune": "info",
};
const MUTED_ROLES = new Set(["label", "section", "metadata", "time", "cardMeta", "cardTime", "mono"]);

// The role frontend-design.md names for a size and weight, or null where the table has no exact row.
const roleFor = (size, weight, mono) => {
  const key = `${size}|${weight ?? ""}|${mono ? "mono" : ""}`;
  return (
    {
      "2xl||": { role: "display" },
      "2xs||mono": { role: "mono" },
      "base||": { role: "body" },
      "base|font-semibold|": { role: "heading" },
      "lg||": { role: "headline" },
      "md||": { role: "reading" },
      "md|font-semibold|": { role: "heading" },
      "sm|font-medium|": { role: "cardLabel" },
      "sm|font-semibold|": { role: "cardLabel", strong: true },
      "sm||": { role: "compact" },
      "xl|font-semibold|": { role: "title" },
      "xs|font-medium|": { role: "label" },
      "xs||": { role: "hint" },
      "xs||mono": { role: "mono" },
    }[key] ?? null
  );
};

const isTypeUtility = (util) =>
  /^text-(?:2xs|xs|sm|base|md|lg|xl|[2-9]xl)$/.test(util) ||
  /^font-/.test(util) ||
  /^leading-/.test(util) ||
  /^tracking-/.test(util) ||
  (/^text-/.test(util) && !TEXT_ALIGN_OR_WRAP.test(util.slice(5)) && !util.startsWith("text-["));

// textPlan turns a plain element's typography classes into Text props, or says why it cannot.
const textPlan = (tokens) => {
  let size = null;
  let weight = null;
  let mono = false;
  let tone = null;
  let uppercase = false;
  let wide = false;
  const consumed = new Set();
  for (const token of tokens) {
    const { util } = token;
    if (!isTypeUtility(util) && util !== "uppercase") continue;
    if (token.variants.length > 0 || token.important || token.negative)
      return { reason: `type class ${token.raw}` };
    if (util === "uppercase") {
      uppercase = true;
      continue;
    }
    if (util === "tracking-wide") {
      wide = true;
      continue;
    }
    if (/^text-(?:2xs|xs|sm|base|md|lg|xl|2xl)$/.test(util)) {
      if (size) return { reason: "two sizes" };
      size = util.slice(5);
    } else if (util === "font-mono") mono = true;
    else if (util === "font-medium" || util === "font-semibold") {
      if (weight) return { reason: "two weights" };
      weight = util;
    } else if (util in TONES) {
      if (tone) return { reason: "two colours" };
      tone = TONES[util];
    } else return { reason: `unmapped type class ${token.raw}` };
    consumed.add(token.raw);
  }
  if (!size || !TEXT_SIZES.has(size)) return { reason: "no size class" };
  const row = roleFor(size, weight, mono);
  if (!row)
    return {
      reason: `no exact role for ${[`text-${size}`, weight, mono && "font-mono"].filter(Boolean).join(" ")}`,
    };
  if (uppercase !== wide) return { reason: "uppercase and tracking-wide are one role only together" };
  const props = [["textRole", `textRole="${row.role}"`]];
  if (row.strong) props.push(["strong", "strong"]);
  if (uppercase && wide) {
    props.push(["shout", "shout"]);
    for (const token of tokens)
      if (token.util === "uppercase" || token.util === "tracking-wide") consumed.add(token.raw);
  }
  const defaultTone = MUTED_ROLES.has(row.role) ? "secondary" : "primary";
  if (tone && tone !== defaultTone) props.push(["tone", `tone="${tone}"`]);
  return { consumed, props };
};

// deadTypography lists the classes on an existing Text that its inline style already overrides.
const deadTypography = (tokens) =>
  tokens
    .filter((token) => token.variants.length === 0 && !token.important && !token.negative)
    .filter((token) => isTypeUtility(token.util) && !/^text-\[/.test(token.util))
    .map((token) => token.raw);

// ---- Source rewriting -----------------------------------------------------------------------------

const attributeName = (attribute) =>
  attribute.type === "JSXAttribute"
    ? attribute.name.type === "JSXIdentifier"
      ? attribute.name.name
      : `${attribute.name.namespace.name}:${attribute.name.name.name}`
    : null;

const classValue = (attribute) => {
  const { value } = attribute;
  if (value?.type === "StringLiteral") return value.value;
  if (value?.type === "JSXExpressionContainer") {
    const { expression } = value;
    if (expression.type === "StringLiteral") return expression.value;
    if (expression.type === "TemplateLiteral" && expression.expressions.length === 0)
      return expression.quasis[0].value.cooked ?? null;
  }
  return null;
};

const elementName = (opening) => (opening.name.type === "JSXIdentifier" ? opening.name.name : null);

const lineOf = (source, offset) => source.slice(0, offset).split("\n").length;

// topLevelBindings names what a module already declares, so an import is never added over a binding.
const topLevelBindings = (program) => {
  const names = new Set();
  const declare = (node) => {
    if (!node) return;
    if (node.type === "Identifier") names.add(node.name);
    else if (node.type === "ObjectPattern")
      for (const property of node.properties) declare(property.value ?? property.argument);
    else if (node.type === "ArrayPattern") for (const element of node.elements) declare(element);
    else if (node.type === "AssignmentPattern") declare(node.left);
  };
  for (const statement of program.body) {
    const declaration = statement.type === "ExportNamedDeclaration" ? statement.declaration : statement;
    if (statement.type === "ImportDeclaration") {
      for (const specifier of statement.specifiers) names.add(specifier.local.name);
    } else if (declaration?.type === "VariableDeclaration") {
      for (const item of declaration.declarations) declare(item.id);
    } else if (declaration?.id) names.add(declaration.id.name);
  }
  return names;
};

const designSystemImport = (program, name) => {
  for (const statement of program.body) {
    if (
      statement.type !== "ImportDeclaration" ||
      statement.source.value !== DESIGN_SYSTEM ||
      statement.importKind === "type"
    )
      continue;
    if (statement.specifiers.some((specifier) => specifier.type === "ImportNamespaceSpecifier"))
      return { has: false, declaration: null };
    if (
      statement.specifiers.some(
        (specifier) =>
          specifier.type === "ImportSpecifier" &&
          specifier.importKind !== "type" &&
          specifier.imported.name === name &&
          specifier.local.name === name,
      )
    ) {
      return { has: true, declaration: statement };
    }
  }
  const declaration = program.body.find(
    (statement) =>
      statement.type === "ImportDeclaration" &&
      statement.source.value === DESIGN_SYSTEM &&
      statement.importKind !== "type" &&
      statement.specifiers.length > 0 &&
      statement.specifiers.every((specifier) => specifier.type === "ImportSpecifier"),
  );
  return { has: false, declaration: declaration ?? null };
};

// ---- Block containers (opt-in) --------------------------------------------------------------------
// A plain block div whose children are all block elements lays out the same as a column Stack, so its
// padding, margin and flex-item classes can move too. Anything that depends on inline flow (text
// classes, `space-y-*`, a child selector) or any child that might be inline keeps the div.

const BLOCK_CHILD_TAGS = new Set([
  "article",
  "aside",
  "blockquote",
  "details",
  "div",
  "dl",
  "fieldset",
  "figure",
  "footer",
  "form",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "header",
  "hr",
  "main",
  "nav",
  "ol",
  "p",
  "pre",
  "section",
  "Stack",
  "table",
  "ul",
]);
const BLOCK_CONTAINER_TAGS = new Set(["aside", "article", "div", "footer", "header", "nav", "section"]);
const INLINE_FLOW_CLASS =
  /^(?:text-|font-|leading-|tracking-|truncate$|whitespace-|break-|line-clamp-|list-|uppercase$|lowercase$|capitalize$|italic$|underline$|no-underline$|line-through$|columns-|float-|clear-|space-[xy]-|divide-|prose|align-|indent-|\*:|\[&)/;

const isBlockChild = (node) => {
  if (!node) return false;
  switch (node.type) {
    case "JSXText":
      return node.value.trim() === "";
    case "JSXElement": {
      const name = elementName(node.openingElement);
      if (name === "Text") {
        const as = node.openingElement.attributes.find(
          (attribute) => attribute.type === "JSXAttribute" && attributeName(attribute) === "as",
        );
        return as?.value?.type === "StringLiteral" && BLOCK_CHILD_TAGS.has(as.value.value);
      }
      return name !== null && BLOCK_CHILD_TAGS.has(name);
    }
    case "JSXFragment":
      return node.children.every(isBlockChild);
    case "JSXExpressionContainer": {
      const { expression } = node;
      if (expression.type === "JSXEmptyExpression") return true;
      if (expression.type === "LogicalExpression" && expression.operator === "&&")
        return isBlockChild(expression.right);
      if (expression.type === "ConditionalExpression") {
        const branch = (side) => side.type === "NullLiteral" || isBlockChild(side);
        return branch(expression.consequent) && branch(expression.alternate);
      }
      if (
        expression.type === "CallExpression" &&
        expression.callee.type === "MemberExpression" &&
        expression.callee.property.name === "map" &&
        expression.arguments[0]?.type === "ArrowFunctionExpression"
      ) {
        return isBlockChild(expression.arguments[0].body);
      }
      return isBlockChild(expression);
    }
    default:
      return false;
  }
};

const isBlockContainer = (element, tokens) =>
  BLOCK_CONTAINER_TAGS.has(elementName(element.openingElement) ?? "") &&
  element.children.some((child) => child.type !== "JSXText" || child.value.trim() !== "") &&
  element.children.every(isBlockChild) &&
  !tokens.some((token) => INLINE_FLOW_CLASS.test(token.util));

// transform returns the rewritten source with a report of what moved and what stayed. With
// `blocks`, a block div of block children is converted as well.
const transform = (source, file = "file.tsx", options = {}) => {
  const ast = parse(source, { attachComment: true, plugins: ["typescript", "jsx"], sourceType: "module" });
  const { program } = ast;
  const bindings = topLevelBindings(program);
  const edits = [];
  const converted = [];
  const left = [];
  const needs = new Set();
  let removed = 0;

  const canUse = (name) => {
    const imported = designSystemImport(program, name);
    return imported.has || !bindings.has(name);
  };
  const commentsWithin = (node) =>
    ast.comments.some((comment) => comment.start >= node.start && comment.end <= node.end);

  const visit = (node) => {
    if (!node || typeof node !== "object") return;
    if (Array.isArray(node)) {
      for (const child of node) visit(child);
      return;
    }
    if (node.type === "JSXElement") rewrite(node);
    for (const [key, child] of Object.entries(node)) {
      if (
        ["loc", "start", "end", "extra", "leadingComments", "trailingComments", "innerComments"].includes(key)
      )
        continue;
      if (child && typeof child === "object") visit(child);
    }
  };

  // attributesWith re-emits an opening tag's attributes with `added` merged in. When the attributes
  // are alphabetical and carry no comment the new ones keep that order; otherwise they sit where the
  // className was.
  const attributesWith = (opening, classAttribute, added, className) => {
    const classText = className === "" ? null : `className=${JSON.stringify(className)}`;
    const compare = (a, b) =>
      a.toLowerCase() < b.toLowerCase() ? -1 : a.toLowerCase() > b.toLowerCase() ? 1 : 0;
    const newTexts = [...added.entries()].sort(([a], [b]) => compare(a, b));
    const plain = opening.attributes.every(
      (attribute) => attribute.type === "JSXAttribute" && attributeName(attribute) !== null,
    );
    const names = opening.attributes.map(attributeName);
    const sorted = plain && names.every((name, index) => index === 0 || compare(names[index - 1], name) <= 0);
    if (added.size === 0)
      return { end: classAttribute.end, start: classAttribute.start, text: classText ?? "" };
    if (sorted && !commentsWithin(opening)) {
      const entries = opening.attributes
        .filter((attribute) => attribute !== classAttribute)
        .map((attribute) => [attributeName(attribute), source.slice(attribute.start, attribute.end)]);
      if (classText) entries.push(["className", classText]);
      for (const [name, text] of newTexts) entries.push([name, text]);
      entries.sort(([a], [b]) => compare(a, b));
      const first = opening.attributes[0].start;
      const last = opening.attributes.at(-1).end;
      return { end: last, start: first, text: entries.map(([, text]) => text).join(" ") };
    }
    const replacement = [...newTexts.map(([, text]) => text), ...(classText ? [classText] : [])].join(" ");
    return { end: classAttribute.end, start: classAttribute.start, text: replacement };
  };

  const rewrite = (element) => {
    const { openingElement: opening, closingElement: closing } = element;
    const name = elementName(opening);
    if (!name) return;
    const isStack = name === "Stack";
    const isText = name === "Text";
    const isNewStack = Object.hasOwn(STACK_TAGS, name);
    const isNewText = TEXT_TAGS.has(name);
    if (!isStack && !isText && !isNewStack && !isNewText) return;
    const classAttribute = opening.attributes.find(
      (attribute) => attribute.type === "JSXAttribute" && attributeName(attribute) === "className",
    );
    if (!classAttribute) return;
    const line = lineOf(source, opening.start);
    const where = `${line}: <${name}>`;
    const text = classValue(classAttribute);
    if (text === null) {
      left.push(`${where} dynamic className`);
      return;
    }
    const tokens = text.split(/\s+/).filter(Boolean).map(parseToken);
    const existing = new Set(opening.attributes.map(attributeName).filter(Boolean));
    const hasSpread = opening.attributes.some((attribute) => attribute.type !== "JSXAttribute");

    if (isText) {
      if (!existing.has("textRole")) return;
      const dead = deadTypography(tokens);
      if (dead.length === 0) return;
      const keep = tokens
        .filter((token) => !dead.includes(token.raw))
        .map((token) => token.raw)
        .join(" ");
      edits.push({ ...attributesWith(opening, classAttribute, new Map(), keep), kind: "attributes" });
      removed += dead.length;
      converted.push(`${where} dropped ${dead.join(" ")} (the role's own style overrides them)`);
      return;
    }

    if (isStack) {
      if (hasSpread) return;
      const plan = layoutPlan(tokens, { existing, isNew: false });
      if (plan.consumed.size === 0) {
        if (plan.reasons.length > 0) left.push(`${where} ${plan.reasons.join("; ")}`);
        return;
      }
      const keep = tokens
        .filter((token) => !plan.consumed.has(token.raw))
        .map((token) => token.raw)
        .join(" ");
      edits.push({ ...attributesWith(opening, classAttribute, plan.props, keep), kind: "attributes" });
      removed += plan.consumed.size;
      converted.push(`${where} moved ${[...plan.consumed].join(" ")}`);
      if (plan.reasons.length > 0) left.push(`${where} ${plan.reasons.join("; ")}`);
      return;
    }

    const flexes = tokens.some((token) => token.util === "flex" && token.variants.length === 0);
    const block =
      Boolean(options.blocks) &&
      isNewStack &&
      !flexes &&
      tokens.some((token) => familyOf(token) !== null) &&
      isBlockContainer(element, tokens);
    if (isNewStack && (flexes || block)) {
      if (hasSpread) {
        left.push(`${where} spread attributes`);
        return;
      }
      const allowed = (attributeKey) =>
        PLAIN_ATTRIBUTES.has(attributeKey) ||
        /^(?:aria-|data-)/.test(attributeKey) ||
        (name === "form" && FORM_ATTRIBUTES.has(attributeKey));
      const stray = [...existing].find((attributeKey) => !allowed(attributeKey));
      if (stray) {
        left.push(`${where} attribute ${stray} is not one a Stack passes`);
        return;
      }
      if (!canUse("Stack")) {
        left.push(`${where} Stack is bound to something else in this file`);
        return;
      }
      const plan = layoutPlan(tokens, { block, existing, isNew: true });
      if (plan.reason) {
        // A block container that has nothing to move is just a div; it is not worth a line.
        if (!block) left.push(`${where} ${plan.reason}`);
        return;
      }
      const render = STACK_TAGS[name];
      const props = new Map(plan.props);
      if (render) props.set("render", `render="${render}"`);
      const keep = tokens
        .filter((token) => !plan.consumed.has(token.raw))
        .map((token) => token.raw)
        .join(" ");
      edits.push({ end: opening.name.end, kind: "name", start: opening.name.start, text: "Stack" });
      if (closing)
        edits.push({ end: closing.name.end, kind: "name", start: closing.name.start, text: "Stack" });
      edits.push({ ...attributesWith(opening, classAttribute, props, keep), kind: "attributes" });
      needs.add("Stack");
      removed += plan.consumed.size;
      converted.push(`${where} → Stack ${[...plan.consumed].join(" ")}`);
      if (plan.reasons.length > 0) left.push(`${where} ${plan.reasons.join("; ")}`);
      return;
    }

    // A plain text element.
    if (!isNewText || hasSpread) return;
    const typography = tokens.filter((token) => isTypeUtility(token.util) || token.util === "uppercase");
    if (typography.length === 0) return;
    const stray = [...existing].find(
      (attributeKey) => !(PLAIN_ATTRIBUTES.has(attributeKey) || /^(?:aria-|data-)/.test(attributeKey)),
    );
    if (stray) {
      left.push(`${where} attribute ${stray} is not one a Text passes`);
      return;
    }
    if (!canUse("Text")) {
      left.push(`${where} Text is bound to something else in this file`);
      return;
    }
    const plan = textPlan(tokens);
    if (plan.reason) {
      left.push(`${where} ${plan.reason}`);
      return;
    }
    const props = new Map([...plan.props, ["as", `as="${name}"`]]);
    const keep = tokens
      .filter((token) => !plan.consumed.has(token.raw))
      .map((token) => token.raw)
      .join(" ");
    edits.push({ end: opening.name.end, kind: "name", start: opening.name.start, text: "Text" });
    if (closing) edits.push({ end: closing.name.end, kind: "name", start: closing.name.start, text: "Text" });
    edits.push({ ...attributesWith(opening, classAttribute, props, keep), kind: "attributes" });
    needs.add("Text");
    removed += plan.consumed.size;
    converted.push(`${where} → Text ${plan.props.map(([, attributeText]) => attributeText).join(" ")}`);
  };

  visit(program);

  // The import the new elements need.
  const missing = [...needs].filter((required) => !designSystemImport(program, required).has);
  if (missing.length > 0) {
    const { declaration } = designSystemImport(program, missing[0]);
    if (declaration) {
      const last = declaration.specifiers.at(-1);
      edits.push({ end: last.end, kind: "import", start: last.end, text: `, ${missing.join(", ")}` });
    } else {
      const lastImport = [...program.body]
        .reverse()
        .find((statement) => statement.type === "ImportDeclaration");
      const text = `import { ${missing.join(", ")} } from "${DESIGN_SYSTEM}";`;
      if (lastImport)
        edits.push({ end: lastImport.end, kind: "import", start: lastImport.end, text: `\n${text}` });
      else edits.push({ end: 0, kind: "import", start: 0, text: `${text}\n` });
    }
  }

  // Apply back to front so every offset still points at the original text. Edits are disjoint:
  // a tag name, an attribute list and an import never overlap, and an element's own edits never reach
  // into its children.
  let output = source;
  for (const edit of [...edits].sort((a, b) => b.start - a.start || b.end - a.end)) {
    output = output.slice(0, edit.start) + edit.text + output.slice(edit.end);
  }
  return { converted, file, left, output, removed };
};

// ---- CLI ------------------------------------------------------------------------------------------

const run = (argv) => {
  const write = argv.includes("--write");
  const patterns = argv.filter((argument) => !argument.startsWith("--"));
  if (patterns.length === 0) {
    console.error("usage: tailwind-to-ds.mjs [--write] [--blocks] <file or glob>...");
    return 2;
  }
  const files = [...new Set(patterns.flatMap((pattern) => globSync(pattern)))]
    .filter((file) => /\.tsx$/.test(file))
    .sort();
  let totalRemoved = 0;
  let changedFiles = 0;
  for (const file of files) {
    const source = readFileSync(file, "utf8");
    let result;
    try {
      result = transform(source, file, { blocks: argv.includes("--blocks") });
    } catch (error) {
      console.error(`${file}: skipped, ${error.message}`);
      continue;
    }
    const changed = result.output !== source;
    if (!changed && result.left.length === 0) continue;
    console.log(
      `${relative(process.cwd(), resolve(file))}${changed ? ` (${result.removed} classes moved)` : ""}`,
    );
    for (const line of result.converted) console.log(`  changed  ${line}`);
    for (const line of result.left) console.log(`  left     ${line}`);
    if (changed) {
      changedFiles += 1;
      totalRemoved += result.removed;
      if (write) writeFileSync(file, result.output);
    }
  }
  console.log(`${write ? "wrote" : "would change"} ${changedFiles} files, ${totalRemoved} classes moved`);
  return 0;
};

if (process.argv[1] === fileURLToPath(import.meta.url)) process.exitCode = run(process.argv.slice(2));

export { transform };
