import { classicEpisode } from "../programmes";

const springfieldChannel = {
  channelLogoState: "missing",
  channelName: "Springfield Classics",
  channelNumber: "1",
  id: "ch-springfield",
  next: { timeLabel: "7:30 PM", title: "Home Sweet Homediddly-Dum-Doodily" },
  now: { ...classicEpisode, channelName: "Springfield Classics", channelNumber: "1" },
} as const;

const actionChannel = {
  channelLogoState: "missing",
  channelName: "1980s Action Heroes",
  channelNumber: "3",
  id: "ch-action",
  next: { timeLabel: "9:05 PM", title: "Point Break" },
  now: {
    artworkState: "missing",
    badge: { label: "On now", tone: "live" },
    progressPercent: 68,
    timeLabel: "7:35–9:05 PM",
    title: "Heat",
  },
} as const;

const trekChannel = {
  channelLogoState: "missing",
  channelName: "Star Trek Classics",
  channelNumber: "2",
  id: "ch-scifi",
  next: { timeLabel: "8:45 PM", title: "Family" },
  now: {
    artworkState: "missing",
    badge: { label: "On now", tone: "live" },
    episodeLabel: "S03E26",
    progressPercent: 31,
    seriesTitle: "Star Trek: TNG",
    timeLabel: "7:15–8:45 PM",
    title: "The Best of Both Worlds",
  },
} as const;

const surfGroups = [
  { channels: [], kind: "favourites", label: "Favourites" },
  { channels: [springfieldChannel], kind: "recent", label: "Recent" },
  {
    channels: [springfieldChannel, trekChannel, actionChannel],
    kind: "all",
    label: "All channels",
  },
] as const;

export { actionChannel, springfieldChannel, surfGroups, trekChannel };
