// languageLabel turns an ISO language code into a human name using the PLATFORM's own language
// data (Intl.DisplayNames) — not a hand-written table that would drift and only cover a handful of
// languages. The mapping is the browser's CLDR data, localized to the viewer's own locale for
// free ("eng" → "English" for an English user, "Englisch" for a German one).
//
// Two realities it handles:
//  - Our tracks carry ISO 639-2 (3-letter, "eng"); Intl prefers 639-1 (2-letter, "en") but accepts
//    many 3-letter codes. Where it can't resolve one it returns the input unchanged, so we fall
//    back to the upper-cased code — "eng" is a worse label than "English" but far better than a
//    crash or a blank.
//  - Intl.DisplayNames throws on a malformed code, so the call is guarded.

// One instance, created lazily and reused — constructing it per call is wasteful, and it must be
// guarded because very old runtimes lack Intl.DisplayNames entirely.
let displayNames: Intl.DisplayNames | undefined;
let initialised = false;

const resolver = (): Intl.DisplayNames | undefined => {
  if (!initialised) {
    initialised = true;
    try {
      displayNames = new Intl.DisplayNames(undefined, { type: "language" });
    } catch {
      displayNames = undefined;
    }
  }
  return displayNames;
};

const languageLabel = (code: string): string => {
  const c = code.trim();
  if (!c) return "Unknown";
  const dn = resolver();
  if (dn) {
    try {
      const name = dn.of(c);
      // Intl returns the input unchanged when it can't resolve the code — treat that as "no match"
      // and fall through to the code fallback rather than showing the raw tag as if it were a name.
      if (name && name.toLowerCase() !== c.toLowerCase()) return name;
    } catch {
      // malformed code — fall through
    }
  }
  return c.toUpperCase();
};

export { languageLabel };
