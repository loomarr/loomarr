// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// The same invented content for every variant, so the maintainer compares grammar, not copy.
import type { StatusTone } from "@loomarr/design-system";

type State = "normal" | "problems";

interface Chan {
  era: string;
  name: string;
  number: string;
}

interface Status {
  label: string;
  tone: StatusTone;
}

interface Programme {
  channel: Chan;
  episode?: string;
  hue: number;
  id: string;
  time: string;
  title: string;
}

interface RequestItem {
  action: string;
  channel?: Chan;
  hue: number;
  id: string;
  meta: string;
  status: Status;
  title: string;
}

interface ClipItem {
  action: string;
  duration: string;
  era: string;
  hue: number;
  id: string;
  kind: string;
  meta: string;
  status?: Status;
  title: string;
}

interface SourceItem {
  action: string;
  icon: "folder" | "archive" | "news";
  id: string;
  kind: string;
  meta: string;
  status: Status;
  title: string;
}

interface PersonItem {
  action: string;
  id: string;
  kind: string;
  meta: string;
  name: string;
  status: Status;
}

const CHANNELS: Record<string, Chan> = {
  scifi: { era: "1980s", name: "Late Night Sci-Fi", number: "03" },
  cartoons: { era: "1990s", name: "Saturday Cartoons", number: "07" },
  movies: { era: "1970s", name: "Classic Movies", number: "12" },
};

const PROGRAMMES: Programme[] = [
  {
    channel: CHANNELS.scifi as Chan,
    episode: "Season 2, episode 5",
    hue: 220,
    id: "p1",
    time: "9:00 – 9:30 PM",
    title: "The Lantern Station",
  },
  {
    channel: CHANNELS.cartoons as Chan,
    episode: "Episode 41",
    hue: 30,
    id: "p2",
    time: "9:00 – 9:30 PM",
    title: "Captain Pogo’s Clubhouse",
  },
  {
    channel: CHANNELS.movies as Chan,
    hue: 160,
    id: "p3",
    time: "8:30 – 10:45 PM",
    title: "Harbor Lights",
  },
];

const requests = (state: State): RequestItem[] => [
  {
    action: state === "problems" ? "Review" : "Open",
    channel: CHANNELS.scifi as Chan,
    hue: 260,
    id: "r1",
    meta: "Requested 2 days ago",
    status:
      state === "problems"
        ? { label: "Waiting for approval", tone: "attention" }
        : { label: "Downloading, 8 of 36 episodes", tone: "progress" },
    title: "The Copper Hills",
  },
  {
    action: "Open channel",
    channel: CHANNELS.cartoons as Chan,
    hue: 20,
    id: "r2",
    meta: "Requested 5 days ago",
    status: { label: "On the schedule", tone: "ok" },
    title: "Professor Mumble’s Mysteries",
  },
  {
    action: "Open",
    hue: 120,
    id: "r3",
    meta: "Requested 1 week ago",
    status: { label: "Ready to watch", tone: "ok" },
    title: "A Quiet Harvest",
  },
];

const clips = (state: State): ClipItem[] => [
  {
    action: "Preview",
    duration: "0:30",
    era: "1990s",
    hue: 15,
    id: "c1",
    kind: "Commercial",
    meta: "Kids · aired 12 times",
    title: "Fizzwell Soda — summer jingle",
  },
  {
    action: state === "problems" ? "Look at it" : "Preview",
    duration: "0:30",
    era: "1980s",
    hue: 200,
    id: "c2",
    kind: "Commercial",
    meta: "Family · never aired",
    ...(state === "problems"
      ? { status: { label: "Failed its quality check", tone: "error" as StatusTone } }
      : {}),
    title: "Cloudy Day Soup — jingle",
  },
  {
    action: "Preview",
    duration: "0:10",
    era: "1990s",
    hue: 80,
    id: "c3",
    kind: "Station ID",
    meta: "General · aired 4 times",
    title: "Station break — five to nine",
  },
  {
    action: "Preview",
    duration: "0:15",
    era: "1990s",
    hue: 300,
    id: "c4",
    kind: "Bumper",
    meta: "Kids · aired 9 times",
    title: "Back after the break",
  },
  {
    action: "Preview",
    duration: "0:45",
    era: "1980s",
    hue: 50,
    id: "c5",
    kind: "Commercial",
    meta: "General · aired 2 times",
    title: "Moonpie Motors — dealership spot",
  },
];

const sources = (state: State): SourceItem[] => [
  {
    action: "Check now",
    icon: "folder",
    id: "s1",
    kind: "Folder",
    meta: "42 clips · checked 2 hours ago",
    status: { label: "Ready", tone: "ok" },
    title: "Drop folder",
  },
  {
    action: "Check now",
    icon: "archive",
    id: "s2",
    kind: "Archive",
    meta: "310 clips · checked 6 hours ago",
    status: { label: "Ready", tone: "ok" },
    title: "Public-domain ad archive",
  },
  {
    action: state === "problems" ? "Try again" : "Check now",
    icon: "news",
    id: "s3",
    kind: "Archive",
    meta: state === "problems" ? "18 clips · last checked 23 hours ago" : "18 clips · checked 1 hour ago",
    status:
      state === "problems"
        ? { label: "Hasn’t checked in for 23 hours", tone: "warn" }
        : { label: "Ready", tone: "ok" },
    title: "Local news breaks",
  },
];

const people = (state: State): PersonItem[] => [
  {
    action: "Manage",
    id: "u1",
    kind: "Admin",
    meta: "Media-server account · seen today",
    name: "Ada Marlowe",
    status: { label: "You", tone: "ok" },
  },
  {
    action: "Manage",
    id: "u2",
    kind: "Member",
    meta: "Local account · seen yesterday",
    name: "Theo Brightwater",
    status: { label: "Active", tone: "ok" },
  },
  {
    action: state === "problems" ? "New invite" : "Manage",
    id: "u3",
    kind: "Member",
    meta: state === "problems" ? "Invited 9 days ago" : "Local account · seen 3 days ago",
    name: "Juniper Oake",
    status:
      state === "problems" ? { label: "Invitation expired", tone: "warn" } : { label: "Active", tone: "ok" },
  },
];

export type { Chan, ClipItem, PersonItem, Programme, RequestItem, SourceItem, State, Status };
export { clips, PROGRAMMES, people, requests, sources };
