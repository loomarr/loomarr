// Where a file name gives way when it does not fit. The `tail` shows last: the extension and the
// end of the name before it. The cut prefers a word start (after a space, dot, underscore or hyphen)
// in the last TAIL_REACH characters; a single last word longer than that may be cut, at a character
// boundary, keeping its last TAIL_MIN characters. A short name is all tail. `head + tail` is always
// the name as drawn (see `visibleFileName`).
//
// Every count and cut is in user-perceived characters (grapheme clusters), so a cut never splits
// an emoji's surrogate pair, a joined or skin-toned emoji, a flag, or a letter from its accent.
const TAIL_MIN = 8;
const TAIL_REACH = 16;
const EXTENSION_MAX = 5;
const SEPARATOR = /^[\s._-]$/u;

type FileNameParts = {
  head: string;
  tail: string;
  /** The tail's length in characters, which sizes the room the head leaves for it. */
  tailLength: number;
};

// Named by its code point: the formatter writes an escaped joiner out as the invisible character.
const ZERO_WIDTH_JOINER = String.fromCodePoint(0x200d);
const REGIONAL_INDICATOR = /^\p{Regional_Indicator}$/u;

// Without Intl.Segmenter, code points are joined back into clusters: a combining mark, variation
// selector, join control or emoji skin tone stays with what comes before it, a zero-width joiner
// holds the next code point too, and two regional indicators make one flag. Rarer clusters (Hangul
// syllables built from jamo, Indic conjuncts) are kept whole only where Intl.Segmenter exists.
const fallbackGraphemes = (text: string): string[] => {
  const clusters: string[] = [];
  let joinNext = false;
  let openFlag = false;
  for (const point of text) {
    const last = clusters.length - 1;
    const indicator = REGIONAL_INDICATOR.test(point);
    if (
      last >= 0 &&
      (joinNext || (indicator && openFlag) || /^[\p{M}\p{Join_Control}\p{Emoji_Modifier}]$/u.test(point))
    ) {
      clusters[last] += point;
      openFlag = false;
    } else {
      clusters.push(point);
      openFlag = indicator;
    }
    joinNext = point === ZERO_WIDTH_JOINER;
  }
  return clusters;
};

// One segmenter for the module, built on first use; a runtime without Intl.Segmenter takes the
// fallback on every call.
let segmenter: Intl.Segmenter | undefined;
const graphemes = (text: string): string[] => {
  const Segmenter = (Intl as { Segmenter?: typeof Intl.Segmenter }).Segmenter;
  if (!Segmenter) return fallbackGraphemes(text);
  segmenter ??= new Segmenter(undefined, { granularity: "grapheme" });
  return Array.from(segmenter.segment(text), (part) => part.segment);
};

// A control character (a newline, a tab, an escape) has no glyph and would break the line, so it is
// drawn as its visible Unicode "control picture" (U+2400 to U+241F, U+2421 for delete).
const visibleFileName = (name: string): string =>
  name.replace(/\p{Cc}/gu, (control) => {
    const code = control.codePointAt(0) ?? 0;
    return code < 0x20
      ? String.fromCodePoint(0x2400 + code)
      : code === 0x7f
        ? String.fromCodePoint(0x2421)
        : String.fromCodePoint(0xfffd);
  });

// An extension is a run of one to EXTENSION_MAX characters after the last dot, holding no space or
// dot ("mkv", "m2ts"); a dot that leads the name (".nomedia") starts no extension.
const extensionStart = (chars: string[]): number => {
  const dot = chars.lastIndexOf(".");
  if (dot <= 0) return chars.length;
  const after = chars.slice(dot + 1);
  const valid = after.length >= 1 && after.length <= EXTENSION_MAX && after.every((c) => !/^[\s.]$/u.test(c));
  return valid ? dot : chars.length;
};

const splitFileName = (name: string): FileNameParts => {
  const chars = graphemes(name);
  const stemEnd = extensionStart(chars);
  if (stemEnd <= TAIL_MIN) return { head: "", tail: name, tailLength: chars.length };

  const at = (cut: number) => ({
    head: chars.slice(0, cut).join(""),
    tail: chars.slice(cut).join(""),
    tailLength: chars.length - cut,
  });
  // The earliest word start inside the reach, so the tail keeps as much of the end as it may.
  for (let i = Math.max(1, stemEnd - TAIL_REACH); i < stemEnd; i += 1) {
    if (SEPARATOR.test(chars[i - 1] ?? "") && !SEPARATOR.test(chars[i] ?? "")) return at(i);
  }
  return at(stemEnd - TAIL_MIN);
};

export type { FileNameParts };
export { splitFileName, visibleFileName };
