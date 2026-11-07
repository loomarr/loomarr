// Tamagui compiles each style prop to a hashed atomic class (`_c-304351090`) and emits the rule
// once in a <style> block, so a rendered element names its styles only through those rules.
// These helpers resolve an element's classes back to their declarations so a test asserts the
// style an element draws, not the hash Tamagui happened to give it.

const escapeRegExp = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** The opening tag of the last element whose text content starts with `text`. */
const tagBefore = (markup: string, text: string): string => {
  const tags = [...markup.matchAll(new RegExp(`<[a-z]+[^>]*>(?=${escapeRegExp(text)})`, "g"))];
  return tags.at(-1)?.[0] ?? "";
};

/** Every declaration the element's atomic classes carry, joined with `;`. */
const declarationsOf = (markup: string, tag: string): string => {
  const classes = (/class="([^"]*)"/.exec(tag)?.[1] ?? "").split(/\s+/).filter(Boolean);
  return classes
    .filter((name) => name.startsWith("_"))
    .flatMap((name) =>
      [...markup.matchAll(new RegExp(`\\.${escapeRegExp(name)}(?![\\w-])[^{}]*\\{([^}]*)\\}`, "g"))].map(
        (match) => match[1] ?? "",
      ),
    )
    .join(";");
};

/** The declarations of the element whose text content starts with `text`. */
const stylesOfText = (markup: string, text: string): string =>
  declarationsOf(markup, tagBefore(markup, text));

// The package compiles without the DOM lib, so the jsdom side is typed structurally.
type LiveElement = {
  classList: Iterable<string>;
  querySelectorAll(selector: string): Iterable<LiveElement>;
};
type LiveDocument = {
  querySelectorAll(selector: string): Iterable<{ textContent: string | null }>;
  styleSheets: Iterable<{ cssRules: Iterable<{ cssText: string }> }>;
};
const liveDocument = (globalThis as unknown as { document: LiveDocument }).document;

/** All CSS text the document holds: stylesheets Tamagui inserted and any <style> tag content. */
const documentCss = (): string =>
  [
    ...[...liveDocument.styleSheets].flatMap((sheet) => [...sheet.cssRules].map((rule) => rule.cssText)),
    ...[...liveDocument.querySelectorAll("style")].map((style) => style.textContent ?? ""),
  ]
    .join("\n")
    .replace(/\s*([:;])\s*/g, "$1");

/** Declarations of a live DOM element's atomic classes, read from the document's stylesheets. */
const declarationsOfElement = (node: object): string => {
  const css = documentCss();
  return [...(node as LiveElement).classList]
    .filter((name) => name.startsWith("_"))
    .flatMap((name) =>
      [...css.matchAll(new RegExp(`\\.${escapeRegExp(name)}(?![\\w-])[^{}]*\\{([^}]*)\\}`, "g"))].map(
        (match) => match[1] ?? "",
      ),
    )
    .join(";");
};

/** Declarations of a live DOM element and every element inside it. */
const declarationsOfTree = (node: object): string => {
  const element = node as LiveElement;
  return [element, ...element.querySelectorAll("*")].map(declarationsOfElement).join(";");
};

export { declarationsOf, declarationsOfElement, declarationsOfTree, stylesOfText, tagBefore };
