// Test helper. Tamagui compiles each style prop to a hashed atomic class (`_c-304351090`) and emits
// the rule once in a <style> block, so a rendered element names its styles only through those rules.
// These helpers resolve an element's classes back to their declarations so a test asserts the
// style an element draws, not the hash Tamagui happened to give it.

const escapeRegExp = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** The opening tag of the last element whose text content starts with `text`. */
const tagBefore = (markup: string, text: string): string => {
  const tags = [...markup.matchAll(new RegExp(`<[a-z]+[^>]*>(?=${escapeRegExp(text)})`, "g"))];
  return tags.at(-1)?.[0] ?? "";
};

/** The declarations, joined with `;`, that the classes in `names` have in `css`. */
const declarationsFor = (css: string, names: Iterable<string>): string =>
  [...names]
    .filter(Boolean)
    .flatMap((name) =>
      [...css.matchAll(new RegExp(`\\.${escapeRegExp(name)}(?![\\w-])[^{}]*\\{([^}]*)\\}`, "g"))].map(
        (match) => match[1] ?? "",
      ),
    )
    .join(";");

/** Every declaration an opening tag's classes (atomic, `is_*`, `font_*`) carry in the markup. */
const declarationsOf = (markup: string, tag: string): string =>
  declarationsFor(markup, (/class="([^"]*)"/.exec(tag)?.[1] ?? "").split(/\s+/));

/** The declarations of the element whose text content starts with `text`. */
const stylesOfText = (markup: string, text: string): string =>
  declarationsOf(markup, tagBefore(markup, text));

/** The markup minus Tamagui's hidden config-revision marker, which is not content. */
const withoutRuntimeMarker = (markup: string): string =>
  markup.replace(/<span aria-hidden="true" hidden="" data-tamagui-config-revision[^<>]*><\/span>/g, "");

// Packages that also target React Native compile without the DOM lib, so the jsdom side is typed
// structurally, and `document` is read only when a live-DOM helper is called.
type LiveElement = {
  classList: Iterable<string>;
  querySelectorAll(selector: string): Iterable<LiveElement>;
};
type LiveDocument = {
  querySelectorAll(selector: string): Iterable<{ textContent: string | null }>;
  styleSheets: Iterable<{ cssRules: Iterable<{ cssText: string }> }>;
};

/** All CSS text the document holds: stylesheets Tamagui inserted and any <style> tag content. */
const documentCss = (): string => {
  const { document } = globalThis as unknown as { document: LiveDocument };
  return [
    ...[...document.styleSheets].flatMap((sheet) => [...sheet.cssRules].map((rule) => rule.cssText)),
    ...[...document.querySelectorAll("style")].map((style) => style.textContent ?? ""),
  ]
    .join("\n")
    .replace(/\s*([:;])\s*/g, "$1");
};

/** Declarations of a live DOM element's atomic classes, read from the document's stylesheets. */
const declarationsOfElement = (node: object): string =>
  declarationsFor(
    documentCss(),
    [...(node as LiveElement).classList].filter((name) => name.startsWith("_")),
  );

/** Declarations of a live DOM element and every element inside it. */
const declarationsOfTree = (node: object): string => {
  const element = node as LiveElement;
  return [element, ...element.querySelectorAll("*")].map(declarationsOfElement).join(";");
};

export {
  declarationsOf,
  declarationsOfElement,
  declarationsOfTree,
  stylesOfText,
  tagBefore,
  withoutRuntimeMarker,
};
