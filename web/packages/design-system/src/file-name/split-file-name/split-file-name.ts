// Where a file name gives way when it does not fit. The `tail` shows last: the extension and the
// end of the name before it. The cut prefers a word start (after a space, dot, underscore or hyphen)
// in the last TAIL_REACH characters; a single last word longer than that may be cut, at a character
// boundary, keeping its last TAIL_MIN characters. A short name is all tail. `head + tail` is always
// the whole name.
//
// Every count and cut is in user-perceived characters (grapheme clusters), so a cut never splits
// an emoji's surrogate pair, a joined emoji, or a letter from its combining accent.
const TAIL_MIN = 8;
const TAIL_REACH = 16;
const EXTENSION_MAX = 5;
const SEPARATOR = /^[\s._-]$/u;

type FileNameParts = { head: string; tail: string };

// Named by its code point: the formatter writes an escaped joiner out as the invisible character.
const ZERO_WIDTH_JOINER = String.fromCodePoint(0x200d);

// Without Intl.Segmenter, code points are joined back into clusters: a combining mark, variation
// selector or join control stays with what comes before it, and a zero-width joiner holds the next
// code point too.
const fallbackGraphemes = (text: string): string[] => {
  const clusters: string[] = [];
  let joinNext = false;
  for (const point of text) {
    const last = clusters.length - 1;
    if (last >= 0 && (joinNext || /^[\p{M}\p{Join_Control}]$/u.test(point))) {
      clusters[last] += point;
    } else {
      clusters.push(point);
    }
    joinNext = point === ZERO_WIDTH_JOINER;
  }
  return clusters;
};

const graphemes = (text: string): string[] => {
  const Segmenter = (Intl as { Segmenter?: typeof Intl.Segmenter }).Segmenter;
  if (!Segmenter) return fallbackGraphemes(text);
  return Array.from(
    new Segmenter(undefined, { granularity: "grapheme" }).segment(text),
    (part) => part.segment,
  );
};

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
  if (stemEnd <= TAIL_MIN) return { head: "", tail: name };

  const at = (cut: number) => ({ head: chars.slice(0, cut).join(""), tail: chars.slice(cut).join("") });
  // The earliest word start inside the reach, so the tail keeps as much of the end as it may.
  for (let i = Math.max(1, stemEnd - TAIL_REACH); i < stemEnd; i += 1) {
    if (SEPARATOR.test(chars[i - 1] ?? "") && !SEPARATOR.test(chars[i] ?? "")) return at(i);
  }
  return at(stemEnd - TAIL_MIN);
};

export type { FileNameParts };
export { splitFileName };
