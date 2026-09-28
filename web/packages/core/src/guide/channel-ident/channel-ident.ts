// The monogram ident (#1659, decision N8): every client draws a channel without a logo as two
// letters on a tinted, hatched square. The rule lives here so the web rail, the shared guide grid
// and native agree on the letters and the colour for the same channel.

/** Brand hues an ident cycles through by channel number (the web mock's order). */
const CHANNEL_IDENT_HUES = ["tune", "suggest", "signal", "onair", "tune"] as const;

type ChannelIdentHue = (typeof CHANNEL_IDENT_HUES)[number];

/** Two letters from the first two words that start with a letter ("Late Night Sci-Fi" → "LN"). */
const monogramOf = (name: string): string => {
  const words = name
    .split(/[\s—–-]+/)
    .map((w) => w.replace(/[^\p{L}\p{N}]/gu, ""))
    .filter(Boolean);
  const alpha = words.filter((w) => /^\p{L}/u.test(w));
  const source = alpha.length > 0 ? alpha : words;
  const [first, second] = source;
  if (!first) return "CH";
  if (!second) return first.slice(0, 2).toUpperCase();
  return ((first[0] ?? "") + (second[0] ?? "")).toUpperCase();
};

const channelIdentHue = (channelNumber: number): ChannelIdentHue =>
  CHANNEL_IDENT_HUES[Math.abs(channelNumber) % CHANNEL_IDENT_HUES.length] ?? "tune";

export type { ChannelIdentHue };
export { CHANNEL_IDENT_HUES, channelIdentHue, monogramOf };
