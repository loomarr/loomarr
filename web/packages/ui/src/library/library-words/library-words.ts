import { formatGuideTime } from "@loomarr/core/guide";
import type { StatusDot } from "@loomarr/design-system";
import type { ComponentProps } from "react";
import type {
  LibraryDetails,
  LibraryHeldRemoval,
  LibraryScan,
} from "../library-settings/library-settings.type";

// The Library's shared plain words (design/library, PLAIN-WORDS.md): a folder is named for what it
// holds, never by its path; counts read the way people say them.

const joinNames = (names: readonly string[]) =>
  names.length <= 1 ? (names[0] ?? "") : `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`;

// "/media/tv" → "TV", "/media/movies" → "Movies", anything else → its last folder name.
const folderName = (path: string) => {
  const last = path.split("/").filter(Boolean).pop() ?? path;
  return last.length <= 2 ? last.toUpperCase() : last.charAt(0).toUpperCase() + last.slice(1);
};

const count = (n: number, one: string, many: string) =>
  `${n.toLocaleString("en-US")} ${n === 1 ? one : many}`;

// "2,406 movies seem to be gone from your Movies folder".
const heldTitle = (held: LibraryHeldRemoval) => {
  const parts = [
    held.movies ? count(held.movies, "movie", "movies") : "",
    held.episodes ? count(held.episodes, "episode", "episodes") : "",
    held.other ? count(held.other, "other file", "other files") : "",
  ].filter(Boolean);
  const one = held.movies + held.episodes + held.other === 1;
  return `${joinNames(parts)} ${one ? "seems" : "seem"} to be gone from your ${folderName(held.path)} folder`;
};

// "That's Amber Hollow, Brightwater, Calder Point and 2,403 more." — what letting them go removes.
// Empty when no title is named (only extras are gone).
const heldNames = (held: LibraryHeldRemoval) => {
  if (held.titles.length === 0) return "";
  const rest = held.titleCount - held.titles.length;
  const names = rest > 0 ? [...held.titles, `${rest.toLocaleString("en-US")} more`] : held.titles;
  return `That's ${joinNames(names)}.`;
};

// "2 files have names Loomarr can't read".
const unreadableTitle = (paths: readonly string[]) =>
  `${count(paths.length, "file has a name", "files have names")} Loomarr can't read`;

// "Rename Ferrow �t� (1999).mkv and Kettle�.mkv using ordinary letters, then check again." Each
// file by its own name: the folder it sits in is plain.
const unreadableNames = (paths: readonly string[]) => {
  const names = paths.slice(0, 3).map((p) => p.split("/").pop() ?? p);
  const rest = paths.length - names.length;
  return `Rename ${joinNames(rest > 0 ? [...names, `${rest.toLocaleString("en-US")} more`] : names)} using ordinary letters, then check again.`;
};

// The three steps of adding media, as the maintainer reads them.
const STEP_NAMES: Record<Exclude<LibraryScan["step"], "idle">, string> = {
  walking: "Looking through your folders",
  reading: "Reading each file",
  lookingUp: "Looking up details",
};
const LAST_STEP = 3;

// Whole minutes left, never below one; past an hour and a half, whole hours.
const timeLeft = (seconds: number) => {
  const minutes = Math.max(1, Math.ceil(seconds / 60));
  return minutes >= 90 ? { n: Math.round(minutes / 60), unit: "hour" } : { n: minutes, unit: "minute" };
};

// "About 9 minutes left in this step"; on the last step, "About 9 minutes left".
const etaSentence = (seconds: number, lastStep: boolean) => {
  const { n, unit } = timeLeft(seconds);
  const left =
    n === 1 ? `About ${unit === "hour" ? "an hour" : "a minute"} left` : `About ${n} ${unit}s left`;
  return lastStep ? left : `${left} in this step`;
};

// "about 9 min left", for the shelf's one line.
const etaShort = (seconds: number) => {
  const { n, unit } = timeLeft(seconds);
  return `about ${n} ${unit === "hour" ? "hr" : "min"} left`;
};

// What the scanner is doing, as one sentence and its dot, and the lines under it while a step runs.
// running is true while an import shows its steps; quiet is an hourly pass that found nothing to
// add, shown as one small line. With anything waiting in Needs you, or a folder in use that held
// nothing (a drive that may not be connected), idle says only "Up to date" (the word sheet's):
// "every title's ready to air" would contradict it (#2779).
const scanHeadline = (scan: LibraryScan, notAllThere = false) => {
  const active = scan.enabled && scan.step !== "idle";
  const quiet = active && scan.quiet;
  const running = active && !quiet;
  const counted = scan.total > 0;
  const percent = running && counted ? Math.min(100, Math.round((scan.done / scan.total) * 100)) : 0;
  const [tone, text]: [ComponentProps<typeof StatusDot>["tone"], string] = !scan.enabled
    ? ["off", "Reading your media folders is turned off"]
    : quiet
      ? ["progress", "Checking your folders…"]
      : running && scan.paused
        ? ["pending", "Paused while someone is watching"]
        : running
          ? ["progress", "Adding your media"]
          : notAllThere
            ? ["ok", "Up to date"]
            : ["ok", "All caught up — every title's ready to air"];
  const step = scan.step === "idle" ? undefined : scan.step;
  const lastStep = scan.stepNumber === LAST_STEP;
  const eta = running && !scan.paused && scan.etaSeconds !== undefined ? scan.etaSeconds : undefined;
  return {
    current: running && !scan.paused ? scan.current : undefined,
    /** A step is moving: the headline's twirl, never while paused. */
    moving: quiet || (running && !scan.paused),
    percent,
    quiet,
    running,
    /** "Step 1 of 3 · Looking through your folders · 112 of 533". */
    stepLine:
      running && step
        ? [
            `Step ${scan.stepNumber} of ${LAST_STEP}`,
            STEP_NAMES[step],
            counted ? `${scan.done.toLocaleString("en-US")} of ${scan.total.toLocaleString("en-US")}` : "",
          ]
            .filter(Boolean)
            .join(" · ")
        : undefined,
    /** "About 9 minutes left in this step", or the wait before there is one; none while paused. */
    timeLine:
      running && !scan.paused
        ? eta !== undefined
          ? etaSentence(eta, lastStep)
          : "Working out how long this takes…"
        : undefined,
    /** The shelf's: "Step 1 of 3 · 21% · about 9 min left". */
    shortLine:
      running && step
        ? [`Step ${scan.stepNumber} of ${LAST_STEP}`, `${percent}%`, eta !== undefined ? etaShort(eta) : ""]
            .filter(Boolean)
            .join(" · ")
        : undefined,
    text,
    tone,
  };
};

// Titles waiting for their details, said only while a retry is due: while step 3 is looking them up
// the progress says so instead. A key problem never shows here — the household has nothing to do
// about it (a release build ships its own key), so only the server's log says it. undefined when
// nothing waits on a retry.
//   line:   "12 titles are waiting on details — trying again at 3:40 PM" (the Library's line)
//   title:  "12 titles are waiting on details" (Settings → Library's row)
//   reason: "Can't reach TMDB — trying again at 3:40 PM", or "Trying again at 3:40 PM"
// The time is told on the household's clock (the Guide's), the device's while that is unknown.
const detailsWaiting = (details: LibraryDetails | undefined, scan: LibraryScan, timeZone?: string) => {
  if (!details?.retryAt || details.waiting === 0 || scan.step === "lookingUp") return undefined;
  const title = `${count(details.waiting, "title", "titles")} ${details.waiting === 1 ? "is" : "are"} waiting on details`;
  const at = formatGuideTime(Date.parse(details.retryAt), timeZone);
  const retry = `trying again at ${at}`;
  const reason = details.unreachable ? `Can't reach TMDB — ${retry}` : `Trying again at ${at}`;
  return { title, line: `${title} — ${retry}`, reason };
};

export {
  detailsWaiting,
  folderName,
  heldNames,
  heldTitle,
  joinNames,
  scanHeadline,
  unreadableNames,
  unreadableTitle,
};
