import { classicEpisode } from "../programmes";

// Channels and titles from the invented demo catalogue (internal/demolibrary), matching the guide
// test card (testcard.ts) channel for channel (#1592).
const cartoonChannel = {
  channelLogoState: "missing",
  channelName: "Cartoon Classics",
  channelNumber: "1",
  id: "ch-cartoons",
  next: { timeLabel: "7:30 PM", title: "A Very Loud Doorbell at the Lighthouse" },
  now: { ...classicEpisode, channelName: "Cartoon Classics", channelNumber: "1" },
} as const;

const actionChannel = {
  channelLogoState: "missing",
  channelName: "1980s Thrillers",
  channelNumber: "3",
  id: "ch-action",
  next: { timeLabel: "9:05 PM", title: "Midnight at the Mapleton" },
  now: {
    artworkState: "missing",
    badge: { label: "On now", tone: "live" },
    progressPercent: 68,
    timeLabel: "7:35–9:05 PM",
    title: "Marigold Protocol",
  },
} as const;

const scifiChannel = {
  channelLogoState: "missing",
  channelName: "Sci-Fi Classics",
  channelNumber: "2",
  id: "ch-scifi",
  next: { timeLabel: "8:45 PM", title: "The Borrowed Antenna" },
  now: {
    artworkState: "missing",
    badge: { label: "On now", tone: "live" },
    episodeLabel: "S03E26",
    progressPercent: 31,
    seriesTitle: "Cold Relay",
    timeLabel: "7:15–8:45 PM",
    title: "Voices on a Dead Channel",
  },
} as const;

const surfGroups = [
  { channels: [], kind: "favourites", label: "Favourites" },
  { channels: [cartoonChannel], kind: "recent", label: "Recent" },
  {
    channels: [cartoonChannel, scifiChannel, actionChannel],
    kind: "all",
    label: "All channels",
  },
] as const;

export { actionChannel, cartoonChannel, scifiChannel, surfGroups };
