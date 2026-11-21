// Where a file name gives way when it does not fit. The `tail` always shows: the extension and the
// end of the name before it, starting at a word (after a space, dot, underscore or hyphen) when one
// starts in the last TAIL_REACH characters. The `head` takes whatever room is left and ends in an
// ellipsis when there is not enough. A name with no separator near its end keeps its last TAIL_MIN
// characters; a short name is all tail and never shortens.
const TAIL_MIN = 8;
const TAIL_REACH = 16;
// An extension is a short run after the last dot that holds no space (".mkv", ".m2ts"), and a dot
// that leads the name (".nomedia") starts no extension.
const EXTENSION = /\.[^.\s]{1,5}$/;
const SEPARATOR = /[\s._-]/;

type FileNameParts = { head: string; tail: string };

const splitFileName = (name: string): FileNameParts => {
  const match = EXTENSION.exec(name);
  const extension = match && match.index > 0 ? match[0] : "";
  const stem = name.slice(0, name.length - extension.length);
  if (stem.length <= TAIL_MIN) return { head: "", tail: name };

  // The earliest word start inside the reach, so the tail keeps as much of the end as it may.
  for (let i = Math.max(1, stem.length - TAIL_REACH); i < stem.length; i += 1) {
    const before = stem.charAt(i - 1);
    const at = stem.charAt(i);
    if (SEPARATOR.test(before) && !SEPARATOR.test(at)) {
      return { head: stem.slice(0, i), tail: stem.slice(i) + extension };
    }
  }
  const cut = stem.length - TAIL_MIN;
  return { head: stem.slice(0, cut), tail: stem.slice(cut) + extension };
};

export type { FileNameParts };
export { splitFileName };
