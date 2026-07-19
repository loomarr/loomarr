// Shared formatters (frontend-design §4.2). "If it came from a machine it's mono"
// (§2.2) — these produce the strings that render in Geist Mono: channel numbers,
// durations, EPG times, relative timestamps. Pure + deterministic (a `now` is
// always injectable) so the visual suite's frozen clock stays honest (§5.2).

// "1h 42m" · "42m" · "0m" — from a millisecond duration.
const formatDuration = (ms: number): string => {
  const totalMin = Math.max(0, Math.round(ms / 60000));
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
};

// Runtime given in minutes (TMDB/Emby convention).
const formatRuntime = (minutes: number): string => formatDuration(minutes * 60000);

// Sub-minute-aware duration for filler clips (a ":30 spot", a ":05 bumper"), where
// the minute rounding above would collapse a 15s bumper and a 45s ad to the same
// "1m". "45s" · "1m 30s" · "2m" — from a millisecond duration.
const formatClipDuration = (ms: number): string => {
  const totalSec = Math.max(0, Math.round(ms / 1000));
  if (totalSec < 60) return `${totalSec}s`;
  const m = Math.floor(totalSec / 60);
  const s = totalSec % 60;
  return s === 0 ? `${m}m` : `${m}m ${s}s`;
};

// Channel number as displayed — mono, never localized.
const channelNumber = (n: number): string => String(n);

// "8:00 PM" for the EPG. Fixed 12-hour, zero-padded minutes, locale-independent.
const formatEpgTime = (t: Instant): string => {
  const d = new Date(msOf(t));
  let h = d.getHours();
  const m = d.getMinutes();
  const ampm = h >= 12 ? "PM" : "AM";
  h = h % 12 || 12;
  return `${h}:${String(m).padStart(2, "0")} ${ampm}`;
};

// A 0–1 ratio as a percentage: 0.87 → "87%". The domain is documented and enforced
// here BECAUSE it is ambiguous: the suggester's confidence is a ratio, while the LLM
// pull's progress is already 0–100. Two conventions with no named helper is how one
// gets rendered as "0%" or "8700%".
const formatPercent = (ratio: number): string => `${Math.round(ratio * 100)}%`;

// An already-0–100 value: 71 → "71%". Named separately rather than overloading
// formatPercent, so a call site declares which convention its number follows.
const formatPercentPoints = (points: number): string => `${Math.round(points)}%`;

// "5 GiB" — VRAM and model sizes. Trivial today; centralized because it renders in the
// model picker's summary AND per-row, and will render again in the Expo app.
const formatGiB = (n: number): string => `${n} GiB`;

// "1 session" · "3 sessions". English-only, matching the rest of the UI copy.
const pluralize = (n: number, singular: string, plural = `${singular}s`): string =>
  `${n} ${n === 1 ? singular : plural}`;

// A point in time as the API expresses it. Most payloads carry ISO strings, but some
// carry Unix ms (session createdAt/expiresAt, now/next startMs/stopMs), and forcing
// every ms-based caller to round-trip through an ISO string just to format it is how
// duplicate local helpers get written.
type Instant = string | number;

const msOf = (t: Instant): number => (typeof t === "number" ? t : new Date(t).getTime());

// "just now" · "3m ago" · "2h ago" · "5d ago". `now` is injectable for tests.
const formatRelative = (t: Instant, now: number = Date.now()): string => {
  const secs = Math.max(0, Math.round((now - msOf(t)) / 1000));
  if (secs < 45) return "just now";
  const mins = Math.round(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  return `${Math.round(hrs / 24)}d ago`;
};

// The forward-looking twin: "in 20m" · "in 3h" · "in 5d" · "expired". Used for session
// expiry, where "expires in 6d" is the number an admin judges a stale session by.
// Deliberately coarse, matching formatRelative — these read as an at-a-glance sense of
// time, not a precise countdown.
const formatUntil = (t: Instant, now: number = Date.now()): string => {
  const secs = Math.round((msOf(t) - now) / 1000);
  if (secs <= 0) return "expired";
  const mins = Math.round(secs / 60);
  if (mins < 60) return `in ${Math.max(1, mins)}m`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `in ${hrs}h`;
  return `in ${Math.round(hrs / 24)}d`;
};

// A settings registry key as a human label: "library.url" → "Library URL",
// "seerr.api_key" → "Seerr API key". The settings API ships `doc` (help text) but no
// display label (config-design §8), so the label is *derived* — defined once here so
// the wizard (13.3) and Settings (13.4) can never drift, and mobile reuses it.
const KEY_ACRONYMS = new Set(["url", "api", "ai", "llm", "ttl", "tmdb", "id", "m3u", "xmltv"]);

const humanizeSettingKey = (key: string): string =>
  key
    .split(/[._]/)
    .map((word, i) => {
      if (KEY_ACRONYMS.has(word)) return word.toUpperCase();
      return i === 0 ? word.charAt(0).toUpperCase() + word.slice(1) : word;
    })
    .join(" ");

export type { Instant };
export {
  channelNumber,
  formatClipDuration,
  formatDuration,
  formatEpgTime,
  formatGiB,
  formatPercent,
  formatPercentPoints,
  formatRelative,
  formatRuntime,
  formatUntil,
  humanizeSettingKey,
  pluralize,
};
