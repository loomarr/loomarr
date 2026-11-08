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

// What the scanner is doing, as one sentence and its dot. reading is true while new files' details
// are being read, which is when a progress bar shows. With anything waiting in Needs you, idle says
// only "Up to date" (the word sheet's): "every title's ready to air" would contradict it (#2779).
const scanHeadline = (scan: LibraryScan, needsYou = false) => {
  const reading = scan.enabled && scan.stage === "reading" && scan.headersTotal > 0;
  const percent = reading ? Math.round((scan.headersDone / scan.headersTotal) * 100) : 0;
  const [tone, text]: [ComponentProps<typeof StatusDot>["tone"], string] = !scan.enabled
    ? ["off", "Reading your media folders is turned off"]
    : reading && scan.paused
      ? ["pending", "Paused while someone is watching"]
      : reading
        ? ["progress", "Adding your media"]
        : needsYou
          ? ["ok", "Up to date"]
          : ["ok", "All caught up — every title's ready to air"];
  return { percent, reading, text, tone };
};

export { folderName, heldTitle, joinNames, scanHeadline };
