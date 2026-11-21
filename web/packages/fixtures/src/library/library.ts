// Invented Library test-card data: the repo is public, so no title or file name is a real one.

/** The wall's sections with their counts, as the underline tabs show them. */
const librarySections = [
  { count: "3,412", countLabel: "3,412 titles", label: "All", value: "all" },
  { count: "2,180", countLabel: "2,180 movies", label: "Movies", value: "movies" },
  { count: "1,232", countLabel: "1,232 shows", label: "Shows", value: "shows" },
  { label: "Collections", value: "collections" },
] as const;

/** File names in the shapes a messy library holds, for the middle-shortening file name. */
const libraryFileNames = {
  long: "Ashgrove.Station.1974.Remastered.Directors.Edition.1080p.BluRay.mkv",
  spaced: "Beacon Moor (1980) - Extended Cut - 2160p HDR.mp4",
  underscored: "cinder_and_vale_s01e04_the_lantern_keeper_extended.m2ts",
  short: "intro.mkv",
  noExtension: "Brass Orchard Behind The Scenes Reel Part Two",
  noSeparator: "thecartographersdaughterfullfeaturerestored.mkv",
  // A right-to-left name (invented Arabic words: "the long film of the family in the garden").
  rightToLeft: "الفيلم الطويل للعائلة في الحديقة النسخة الكاملة.mkv",
  // A last word longer than the tail: the cut may fall inside it, at a character boundary.
  longLastWord: "Calder.Pier.Lanternwickthistlebrookmeadowmere.mkv",
  // A newline inside the name, as a careless rename leaves it: drawn as a visible picture.
  controlCharacter: "Beacon Moor\nreel two.mkv",
  // Wide glyphs, which fall back to wider fonts than the data face: the extension still shows.
  emoji: "🌊🏔️🎬 Calder Pier 🍿🎞️ summer reel 🌅🌙.mkv",
  // Invented Japanese words ("the long journey of the lighthouse keeper, full edition").
  cjk: "灯台守の長い旅路と海辺の町の物語完全版.mkv",
} as const;

/** Tab labels long enough that the underline tabs scroll sideways at TV density or large type. */
const librarySectionsLong = [
  { count: "3,412", countLabel: "3,412 titles", label: "Everything in the library", value: "all" },
  { count: "2,180", countLabel: "2,180 movies", label: "Feature-length movies", value: "movies" },
  { count: "1,232", countLabel: "1,232 shows", label: "Series and their seasons", value: "shows" },
  { label: "Household collections", value: "collections" },
] as const;

/** A title search as someone types it. */
const libraryQuery = "calder pier";

export { libraryFileNames, libraryQuery, librarySections, librarySectionsLong };
