// What a poster with no artwork draws (design/library): a quiet colour field picked from the title,
// under its monogram.

// Two neutral surface steps. A state's surface would read as that state beside the wall's warn dot,
// and both steps keep the secondary monogram past 4.5:1 in either theme.
const POSTER_TONES = ["surfaceRaised", "surfaceElevated"] as const;

// FNV-1a over the seed, then a final mix so the pick depends on every bit and neighbouring ids
// spread out. A title keeps its colour on every visit.
const posterTone = (seed: string): (typeof POSTER_TONES)[number] => {
  let hash = 0x811c9dc5;
  for (let i = 0; i < seed.length; i++) {
    hash = Math.imul(hash ^ seed.charCodeAt(i), 0x01000193) >>> 0;
  }
  hash = Math.imul(hash ^ (hash >>> 16), 0x85ebca6b) >>> 0;
  hash = (hash ^ (hash >>> 13)) >>> 0;
  return POSTER_TONES[hash % POSTER_TONES.length] ?? POSTER_TONES[0];
};

// The first letter or digit of the first two words, past a leading "The". A title nobody has fixed
// yet can be a file name, so dots and underscores part words too.
const posterMonogram = (name: string) => {
  const words = name.split(/[\s._-]+/).filter((word) => /[\p{L}\p{N}]/u.test(word));
  const telling = words.length > 1 && words[0]?.toLowerCase() === "the" ? words.slice(1) : words;
  return telling
    .slice(0, 2)
    .map((word) => word.match(/[\p{L}\p{N}]/u)?.[0]?.toUpperCase() ?? "")
    .join("");
};

export { POSTER_TONES, posterMonogram, posterTone };
