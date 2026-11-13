import type { LibraryOrderChoice, LibraryOrderKind, LibraryOrderNeed } from "../library-needs.type";
import { plural } from "../plural";
import type { OrderOption } from "./order-options.type";

// ── Episode order (#2151 decision 53, the approved mock in project/prototypes) ──────────────────
// TMDB lists some shows in more than one order (its episode groups). Each is named in plain words
// by its kind; TMDB's own name is kept for a kind with no plain name, or when two share one.

const PLAIN_ORDER: Partial<Record<LibraryOrderKind, string>> = {
  absolute: "One long run",
  dvd: "DVD order",
  digital: "Digital order",
  story: "Story order",
  production: "Production order",
  tv: "TV order",
};

const FILES = "files";
const choiceKey = (c: LibraryOrderChoice) =>
  c.choice === "catalog" ? "catalog" : `order:${c.orderId ?? ""}`;

const orderOptions = (need: LibraryOrderNeed): OrderOption[] => {
  const kinds = need.choices.map((c) => c.kind);
  const options = need.choices.map((c): OrderOption => {
    const shared = kinds.filter((k) => k === c.kind).length > 1;
    const plain = c.kind && !shared ? PLAIN_ORDER[c.kind] : undefined;
    const seasons = c.seasons.filter((s) => s.season > 0 && s.episodes > 0).length;
    const episodes = plural(c.episodes, "episode");
    const sub =
      c.kind === "absolute"
        ? `${episodes}, no seasons`
        : seasons > 0
          ? `${plural(seasons, "season")} · ${episodes}`
          : episodes;
    if (c.choice === "catalog")
      return {
        key: choiceKey(c),
        answer: { choice: "catalog" },
        name: "As first aired",
        sub,
        fits: c.fits,
        done: "uses the order it first aired in",
      };
    const name = plain ?? c.name ?? "Another order";
    return {
      key: choiceKey(c),
      answer: { choice: "order", orderId: c.orderId ?? "" },
      name,
      sub,
      fits: c.fits,
      done: c.kind === "absolute" && plain ? "plays as one long run" : `uses ${name}`,
    };
  });
  options.push({
    key: FILES,
    answer: { choice: "files" },
    name: "Keep the file names",
    sub: "Each episode keeps the name and number in its file. We won't ask again.",
    fits: false,
    done: "keeps the names in its files",
  });
  return options;
};

// One order besides the files' own: nothing to choose, only that a season doesn't line up.
const onlyCatalog = (need: LibraryOrderNeed) => need.choices.length <= 1;

// The first season on disk that doesn't end where TMDB's does, said in a line.
const mismatchLine = (need: LibraryOrderNeed): string => {
  const waiting = need.files.length;
  const sizes = new Map((need.choices[0]?.seasons ?? []).map((s) => [s.season, s.episodes]));
  const off = need.disk.find((d) => sizes.get(d.season) !== d.episodes);
  const one = waiting === 1;
  const still = one
    ? "It still plays, named as it is in its file."
    : `Those ${waiting} still play, named as they are in their files.`;
  if (!off)
    return one
      ? "1 episode still plays, named as it is in its file."
      : `${waiting} episodes still play, named as they are in their files.`;
  const listed = sizes.get(off.season) ?? 0;
  return listed === 0
    ? `Season ${off.season} on your disk isn't one TMDB lists. ${still}`
    : `Season ${off.season} on your disk has ${plural(off.episodes, "episode")}; TMDB lists ${listed}. ${still}`;
};

const orderLine = (need: LibraryOrderNeed) =>
  onlyCatalog(need)
    ? `${need.name} — ${need.files.length === 1 ? "1 episode keeps its file name" : `${need.files.length} episodes keep their file names`}`
    : `${need.name} — which order are the episodes in?`;

export { FILES, mismatchLine, onlyCatalog, orderLine, orderOptions };
