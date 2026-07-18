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
const formatEpgTime = (iso: string): string => {
  const d = new Date(iso);
  let h = d.getHours();
  const m = d.getMinutes();
  const ampm = h >= 12 ? "PM" : "AM";
  h = h % 12 || 12;
  return `${h}:${String(m).padStart(2, "0")} ${ampm}`;
};

// "just now" · "3m ago" · "2h ago" · "5d ago". `now` is injectable for tests.
const formatRelative = (iso: string, now: number = Date.now()): string => {
  const then = new Date(iso).getTime();
  const secs = Math.max(0, Math.round((now - then) / 1000));
  if (secs < 45) return "just now";
  const mins = Math.round(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  return `${Math.round(hrs / 24)}d ago`;
};

export { channelNumber, formatClipDuration, formatDuration, formatEpgTime, formatRelative, formatRuntime };
