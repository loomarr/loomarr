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

/** The markup minus Tamagui's hidden config-revision marker, which is not content. */
const withoutRuntimeMarker = (markup: string): string =>
  markup.replace(/<span aria-hidden="true" hidden="" data-tamagui-config-revision[^>]*><\/span>/g, "");

export { declarationsOf, stylesOfText, tagBefore, withoutRuntimeMarker };
