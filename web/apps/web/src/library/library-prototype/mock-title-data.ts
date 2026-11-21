// PROTOTYPE — throwaway (#4476 A1). Invented title pages, drawing only data the library tables
// already hold (#4476, B1 section B): name, year, rating, length, score, tagline, overview, genres,
// studios, countries, premiere, credits (photos not imported yet: initials), channels, episodes
// with summary, length and still, files with streams, chapters, edition and extra copies.

interface Person {
  name: string;
  role: string;
  /** Whether a photo has been imported; without one the page shows initials. */
  photo?: number;
}

interface Track {
  language: string;
  detail: string;
}

interface MockFile {
  name: string;
  folder: string;
  size: string;
  picture: string;
  sound: string;
  subtitles: string;
  length: string;
  more: {
    container: string;
    video: string;
    bitrate: string;
    frameRate: string;
    hdr: string;
    audio: Track[];
    subs: Track[];
    chapters: string[];
  };
}

interface Episode {
  n: number;
  name: string;
  summary: string;
  length: string;
  hue: number;
  ready?: boolean;
  unplayable?: boolean;
}

interface MockTitlePage {
  kind: "movie" | "show" | "home";
  name: string;
  year: string;
  length: string;
  rating?: string;
  genres: string[];
  score?: string;
  tagline?: string;
  overview?: string;
  facts: [string, string][];
  cast: Person[];
  crew: [string, string][];
  channels: { name: string; number: number }[];
  hue: number;
  hue2: number;
  file: MockFile;
  extra?: MockFile;
  seasons?: { label: string; note?: string; episodes: Episode[] }[];
}

const FILE: MockFile = {
  name: "The Saltmarsh Lantern (1979).mkv",
  folder: "Movies/The Saltmarsh Lantern (1979)",
  size: "8.2 GB",
  picture: "1080p HD",
  sound: "5.1 surround, English",
  subtitles: "English, French",
  length: "1 h 52 min",
  more: {
    container: "Matroska (MKV)",
    video: "H.264, High profile, 8-bit",
    bitrate: "9.8 Mb/s",
    frameRate: "23.976 frames a second",
    hdr: "None (standard range)",
    audio: [
      { language: "English", detail: "5.1 surround · AC-3 · 640 kb/s · default" },
      { language: "English", detail: "Stereo · AAC · director's commentary" },
      { language: "French", detail: "Stereo · AAC" },
    ],
    subs: [
      { language: "English", detail: "Full · default" },
      { language: "English", detail: "For the hard of hearing" },
      { language: "French", detail: "Forced (foreign dialogue only) · separate file, .fr.srt" },
    ],
    chapters: ["0:00 Low tide", "14:20 The keeper's house", "38:05 Fog bank", "1:02:40 The second lamp", "1:31:10 High water"],
  },
};

const MOVIE: MockTitlePage = {
  kind: "movie",
  name: "The Saltmarsh Lantern",
  year: "1979",
  length: "1 h 52 min",
  rating: "PG",
  genres: ["Mystery", "Drama"],
  score: "7.4",
  tagline: "The tide keeps every secret but one.",
  overview:
    "When the lamp on a disused marsh lighthouse starts burning again, a retired harbour pilot and her nephew row out to find who lit it, and find a logbook that ends the night her husband vanished.",
  facts: [
    ["Studio", "Harrowgate Pictures"],
    ["Country", "United Kingdom"],
    ["Released", "14 March 1979"],
  ],
  cast: [
    { name: "Imelda Varge", role: "Agnes Fell", photo: 1 },
    { name: "Tobin Ashcroft", role: "Rafe Fell", photo: 3 },
    { name: "Corin Delahaye", role: "The Keeper" },
    { name: "Wren Okafor-Lisle", role: "Constable Hobb", photo: 0 },
    { name: "Mattias Brule", role: "Ned Callow" },
    { name: "Petra Sunderland", role: "Mrs. Callow", photo: 4 },
    { name: "Osric Hale", role: "Ferryman" },
  ],
  crew: [
    ["Directed by", "Lorna Pettigrew"],
    ["Written by", "Lorna Pettigrew, Abel Strand"],
  ],
  channels: [
    { name: "Late Show", number: 7 },
    { name: "Mystery Hour", number: 12 },
  ],
  hue: 0,
  hue2: 3,
  file: FILE,
  extra: {
    ...FILE,
    name: "Saltmarsh.Lantern.1979.720p.mkv",
    folder: "Movies/Unsorted",
    size: "1.4 GB",
    picture: "720p",
    sound: "Stereo, English",
    subtitles: "None",
  },
};

const ep = (n: number, name: string, summary: string, length: string, extra: Partial<Episode> = {}): Episode => ({
  n,
  name,
  summary,
  length,
  hue: n % 5,
  ...extra,
});

const SERIES: MockTitlePage = {
  kind: "show",
  name: "Copperline",
  year: "2016–2019",
  length: "3 seasons",
  rating: "TV-14",
  genres: ["Crime", "Drama"],
  score: "8.1",
  tagline: "Every wire leads somewhere.",
  overview:
    "A night-shift telephone engineer in a river town starts hearing calls on lines that were cut years ago, and the voices know where the town's money went.",
  facts: [
    ["Network", "Northwire"],
    ["Country", "Ireland"],
    ["First aired", "2 October 2016"],
  ],
  cast: [
    { name: "Saoirse Linnane", role: "Maeve Corr", photo: 2 },
    { name: "Dov Abernathy", role: "Sgt. Pell", photo: 0 },
    { name: "Ruairí Fenwick", role: "Danny Corr" },
    { name: "Hester Quaile", role: "Councillor Breen", photo: 3 },
    { name: "Ines Matarazzo", role: "Lou" },
  ],
  crew: [["Created by", "Fergal Dunleavy"]],
  channels: [{ name: "Late Show", number: 7 }],
  hue: 1,
  hue2: 4,
  file: { ...FILE, name: "Copperline S02E01.mkv", folder: "Shows/Copperline/Season 2", size: "1.9 GB", length: "47 min" },
  seasons: [
    {
      label: "Season 1",
      episodes: [
        ep(1, "Dead Line", "Maeve takes the night shift and answers a call from a disconnected exchange.", "52 min"),
        ep(2, "Party Line", "Three voices on one cut line agree on a date nobody in town remembers.", "48 min"),
        ep(3, "Crossed Wires", "Danny finds a junction box that isn't on any map.", "47 min"),
        ep(4, "Ringback", "Sgt. Pell asks Maeve why the exchange logs show her calling herself.", "49 min", { ready: false }),
        ep(5, "Long Distance", "A call from the drowned mill points at the council's old accounts.", "51 min"),
      ],
    },
    {
      label: "Season 2",
      episodes: [
        ep(1, "Exchange", "A new switchboard goes in and every old line goes quiet but one.", "47 min"),
        ep(2, "Toll", "Lou traces a payment to a company that closed in 1974.", "46 min", { unplayable: true }),
        ep(3, "Engaged", "Breen's campaign rally is interrupted by a call to every phone in the hall.", "48 min", { unplayable: true }),
      ],
    },
    {
      label: "Specials",
      note: "Specials air between seasons, so Loomarr plays them after the season they follow.",
      episodes: [ep(1, "The Christmas Line", "A one-off Christmas Eve shift, set between seasons 1 and 2.", "58 min")],
    },
  ],
};

const HOME: MockTitlePage = {
  kind: "home",
  name: "Summer at the lake",
  year: "2019",
  length: "24 min",
  genres: ["Home videos"],
  facts: [],
  cast: [],
  crew: [],
  channels: [],
  hue: 2,
  hue2: 4,
  file: {
    ...FILE,
    name: "summer at the lake 2019.mp4",
    folder: "Movies/Family",
    size: "2.1 GB",
    picture: "1080p HD",
    sound: "Stereo",
    subtitles: "None",
    length: "24 min",
  },
};

export type { Episode, MockFile, MockTitlePage, Person };
export { HOME, MOVIE, SERIES };
