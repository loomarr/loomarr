// Invented Library test-card data: the repo is public, so no title or file name is a real one.

/** The wall's sections with their counts, as the underline tabs show them. */
const librarySections = [
  { count: "3,412", label: "All", value: "all" },
  { count: "2,180", label: "Movies", value: "movies" },
  { count: "1,232", label: "Shows", value: "shows" },
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
} as const;

/** A title search as someone types it. */
const libraryQuery = "calder pier";

export { libraryFileNames, libraryQuery, librarySections };
