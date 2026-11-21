// PROTOTYPE — throwaway (#4476 A1). An invented catalogue: no real titles, no real paths. The
// counts are the 3,400-title library the controls are designed for; the 60 drawn titles are the
// first screens of it.

type Kind = "movie" | "show" | "home";

interface MockTitle {
  id: string;
  name: string;
  year: number;
  kind: Kind;
  genres: string[];
  /** Days since it was added, for "Recently added". */
  added: number;
  /** Stand-in artwork: which token hue tints it, and which of the poster layouts it uses. */
  hue: number;
  hue2: number;
  layout: number;
}

const NAMES: [string, number, Kind, string][] = [
  ["Ashgrove Station", 1974, "movie", "Drama"],
  ["The Brass Orchard", 2009, "movie", "Mystery"],
  ["Blue Hour at Calder Pier", 1988, "movie", "Romance"],
  ["Copperline", 2016, "show", "Crime"],
  ["The Cartographer's Daughter", 2003, "movie", "Adventure"],
  ["Cinder & Vale", 2021, "show", "Fantasy"],
  ["Dovetail Road", 1995, "movie", "Comedy"],
  ["The Drowned Bell", 1962, "movie", "Mystery"],
  ["Eastwick Lanterns", 2012, "show", "Drama"],
  ["An Ember Kept", 1981, "movie", "Drama"],
  ["Foxglove Terrace", 2019, "show", "Comedy"],
  ["The Fourth Lighthouse", 1957, "movie", "Adventure"],
  ["Glassworks", 2007, "movie", "Thriller"],
  ["Greyfeather", 1999, "show", "Animation"],
  ["Harrow & Finch", 2014, "show", "Crime"],
  ["The Hollow Regatta", 1977, "movie", "Comedy"],
  ["Ironbridge Nights", 1986, "movie", "Thriller"],
  ["Juniper Line", 2022, "show", "Family"],
  ["Kettle Island", 1969, "movie", "Family"],
  ["The Long Quiet", 2011, "movie", "Science fiction"],
  ["Lantern Weather", 1992, "movie", "Romance"],
  ["Marrowby Junction", 2005, "show", "Mystery"],
  ["The Midnight Orrery", 1983, "movie", "Science fiction"],
  ["Moth & Meridian", 2018, "movie", "Drama"],
  ["Northlight Bakery", 2020, "show", "Comedy"],
  ["The Ninth Harbour", 1971, "movie", "Adventure"],
  ["Old Tom's Almanac", 1964, "show", "Family"],
  ["Paper Comets", 1997, "movie", "Animation"],
  ["Pellmore Hall", 2013, "show", "Drama"],
  ["The Quarry Choir", 1989, "movie", "Drama"],
  ["Rainmaker's Ledger", 2001, "movie", "Western"],
  ["Rookwood Below", 2015, "show", "Thriller"],
  ["The Saltmarsh Lantern", 1979, "movie", "Mystery"],
  ["Silverfin Bay", 1994, "show", "Family"],
  ["Small Hours in Tolland", 2010, "movie", "Comedy"],
  ["The Starling Accord", 2024, "movie", "Science fiction"],
  ["Tallow & Thread", 1958, "movie", "Romance"],
  ["Thistledown", 2002, "show", "Animation"],
  ["Under the Lindworth", 1985, "movie", "Fantasy"],
  ["The Velvet Switchboard", 1966, "movie", "Comedy"],
  ["Vesper Fields", 2008, "movie", "Western"],
  ["Wexley Point", 2017, "show", "Crime"],
  ["The Whistling Gate", 1973, "movie", "Family"],
  ["Winter in Vell", 1998, "movie", "Drama"],
  ["Wrenfield Radio", 2023, "show", "Comedy"],
  ["Yarrow Street", 1990, "show", "Drama"],
  ["Zephyr Lane Detectives", 1976, "show", "Crime"],
  ["A Kite for Odile", 2006, "movie", "Family"],
  ["Beacon Moor", 1980, "movie", "Thriller"],
  ["The Clockmaker's Fog", 1951, "movie", "Mystery"],
  ["Driftwood Parade", 2019, "movie", "Animation"],
  ["Elsinmoor", 2015, "show", "Fantasy"],
  ["The Fernhill Tapes", 1993, "movie", "Thriller"],
  ["Gullwing Express", 1968, "movie", "Adventure"],
  ["Halfmoon Diner", 2004, "show", "Comedy"],
  ["The Inkwell Society", 1987, "movie", "Mystery"],
  ["Larkspur Coast", 2021, "movie", "Romance"],
  ["Morrow's Edge", 1960, "movie", "Western"],
  ["Nettlebed Farm", 2012, "show", "Family"],
  ["The Orrin Variations", 2000, "movie", "Drama"],
];

// A stable hash of the name picks the stand-in art, so neighbours on the wall rarely match.
const seed = (s: string) => [...s].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 7);

const TITLES: MockTitle[] = NAMES.map(([name, year, kind, genre], i) => ({
  id: `t${i}`,
  name,
  year,
  kind,
  genres: [genre],
  added: (i * 37) % 400,
  hue: seed(name) % 5,
  hue2: (seed(name) % 5) + 1 + ((seed(`${name}!`) % 3)),
  layout: 0,
}));

// Two home videos: drawn only under the "It's ours" proposal.
const HOME_VIDEOS: MockTitle[] = [
  { id: "h1", name: "Summer at the lake", year: 2019, kind: "home", genres: ["Home videos"], added: 3, hue: 2, hue2: 4, layout: 0 },
  { id: "h2", name: "Grandad's 80th", year: 2022, kind: "home", genres: ["Home videos"], added: 5, hue: 4, hue2: 1, layout: 0 },
];

// The library the controls are designed for.
const COUNTS = { all: 3412, movies: 2180, shows: 1232, collections: 0, ready: 2172 } as const;

const GENRES: [string, number][] = [
  ["Drama", 812],
  ["Comedy", 604],
  ["Family", 377],
  ["Mystery", 341],
  ["Crime", 296],
  ["Thriller", 270],
  ["Animation", 214],
  ["Adventure", 188],
  ["Romance", 133],
  ["Science fiction", 97],
  ["Fantasy", 52],
  ["Western", 28],
];

interface NeedsItem {
  id: string;
  file: string;
  folder: string;
  /** The best guess, when there is one. */
  guess?: { name: string; year: number; why: string; hue: number; hue2: number; layout: number };
  /** The one sentence when there is no guess. */
  none?: string;
  extraFiles?: number;
}

const NEEDS: NeedsItem[] = [
  {
    id: "n1",
    file: "Saltmarsh.Lantern.1979.DVDRip.mkv",
    folder: "Movies/Unsorted",
    guess: { name: "The Saltmarsh Lantern", year: 1979, why: "the name and year match, and so does the length", hue: 3, hue2: 1, layout: 1 },
  },
  {
    id: "n2",
    file: "ferry_night_87.avi",
    folder: "Movies/From old drive",
    guess: { name: "Night Ferry to Orla", year: 1987, why: "the name is close and the year matches; the length is 4 minutes shorter", hue: 0, hue2: 2, layout: 2 },
  },
  {
    id: "n3",
    file: "holiday tape final v2.mp4",
    folder: "Movies/Family",
    none: "Nothing in the catalogue has a name like this. Pick the title yourself, or ignore it.",
  },
  {
    id: "n4",
    file: "VTS_01_1.VOB",
    folder: "Movies/DVD copies/DISC 3",
    none: "The file name is a DVD part, not a title, so there's nothing to search for. Pick the title yourself.",
    extraFiles: 4,
  },
  {
    id: "n5",
    file: "Copperline S02E07.mkv",
    folder: "Shows/Copperline",
    guess: { name: "Copperline", year: 2016, why: "the show matches, but season 2 only has 6 episodes", hue: 1, hue2: 3, layout: 3 },
  },
  {
    id: "n6",
    file: "lindworth_extended_cut.mkv",
    folder: "Movies/Unsorted",
    guess: { name: "Under the Lindworth", year: 1985, why: "the name matches; the year isn't in the file name", hue: 5, hue2: 0, layout: 0 },
  },
];

const IGNORED = [
  { id: "i1", file: "sample.mkv", folder: "Movies/Glassworks (2007)", when: "Ignored 2 days ago" },
  { id: "i2", file: "trailer-final.mp4", folder: "Movies/Unsorted", when: "Ignored last week" },
  { id: "i3", file: "test encode 2.mkv", folder: "Movies/Unsorted", when: "Ignored in October" },
];

const READY_MADE = [
  { name: "The Harbour films", have: 3, of: 4, hues: [3, 1, 5] },
  { name: "Copperline and friends", have: 5, of: 9, hues: [0, 2, 4] },
  { name: "Saturday morning animation", have: 7, of: 12, hues: [4, 5, 1] },
];

export type { Kind, MockTitle, NeedsItem };
export { COUNTS, GENRES, HOME_VIDEOS, IGNORED, NEEDS, READY_MADE, TITLES };

// Compositions follow the A–Z order the wall opens on, so no composition repeats more than twice in
// the first two rows (twelve posters, eight compositions).
const sortName = (s: string) => s.replace(/^(The|An?) /, "");
[...TITLES]
  .sort((a, b) => sortName(a.name).localeCompare(sortName(b.name)))
  .forEach((t, i) => {
    t.layout = (i * 3) % 8;
  });
