// The Watch page's channel drawer rows (#1659 W1): every channel with what is on now and the next
// two hours as a strip of blocks, as the guide reads them. Plain data in the drawer's own row shape,
// so the fixtures package keeps no dependency on the app. Programme names come from the invented
// demo catalogue; no clock is read.

interface DrawerFixtureBlock {
  key: string;
  label?: string;
  minutes: number;
  pod: boolean;
}

interface DrawerFixtureChannel {
  id: string;
  number: number;
  name: string;
  now?: string;
  minutesLeft?: number;
  offAir?: "building" | "paused";
  blocks: DrawerFixtureBlock[];
}

const drawerChannels: DrawerFixtureChannel[] = [
  {
    id: "ch-saturday",
    number: 3,
    name: "Saturday Mornings",
    now: "Gloopers · The Great Gloop",
    minutesLeft: 12,
    blocks: [
      { key: "a1", label: "Gloopers", minutes: 25, pod: false },
      { key: "a2", minutes: 3, pod: true },
      { key: "a3", label: "Captain Zip", minutes: 30, pod: false },
      { key: "a4", minutes: 3, pod: true },
      { key: "a5", label: "Rocket Ranch", minutes: 59, pod: false },
    ],
  },
  {
    id: "ch-thrillers",
    number: 7,
    name: "1980s Thrillers",
    now: "Marigold Protocol",
    minutesLeft: 35,
    blocks: [
      { key: "b1", label: "Marigold Protocol", minutes: 60, pod: false },
      { key: "b2", minutes: 4, pod: true },
      { key: "b3", label: "Night Ledger", minutes: 56, pod: false },
    ],
  },
  {
    id: "ch-scifi",
    number: 12,
    name: "Late Night Sci-Fi",
    now: "Static Orchard",
    minutesLeft: 48,
    blocks: [
      { key: "c1", label: "Static Orchard", minutes: 90, pod: false },
      { key: "c2", minutes: 3, pod: true },
      { key: "c3", label: "Moon Pier", minutes: 27, pod: false },
    ],
  },
  { id: "ch-news", number: 21, name: "Newsreel", offAir: "paused", blocks: [] },
  { id: "ch-new", number: 24, name: "Weekend Matinee", offAir: "building", blocks: [] },
  {
    id: "ch-sitcoms",
    number: 30,
    name: "Sitcom Row",
    now: "Pardon My Porch",
    minutesLeft: 7,
    blocks: [
      { key: "d1", label: "Pardon My Porch", minutes: 30, pod: false },
      { key: "d2", minutes: 2, pod: true },
      { key: "d3", label: "The Neighbours Upstairs", minutes: 30, pod: false },
      { key: "d4", minutes: 2, pod: true },
      { key: "d5", label: "Pardon My Porch", minutes: 56, pod: false },
    ],
  },
];

export type { DrawerFixtureBlock, DrawerFixtureChannel };
export { drawerChannels };
