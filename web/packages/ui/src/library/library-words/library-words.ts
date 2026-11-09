import type { StatusDot } from "@loomarr/design-system";
import type { ComponentProps } from "react";
import type { LibraryHeldRemoval, LibraryScan } from "../library-settings/library-settings.type";

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
// add, shown as one small line. With anything waiting in Needs you, idle says only "Up to date"
// (the word sheet's): "every title's ready to air" would contradict it (#2779).
const scanHeadline = (scan: LibraryScan, needsYou = false) => {
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
          : needsYou
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

export { folderName, heldTitle, joinNames, scanHeadline };
