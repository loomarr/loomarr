// Titles come from the invented demo catalogue (internal/demolibrary); episode names are invented
// in its style. The repo is public, so no real title belongs here (#1592).
const classicEpisode = {
  artworkState: "ready",
  badge: { label: "On now", tone: "live" },
  channelLogoState: "missing",
  channelName: "Classic Animation",
  channelNumber: "07",
  description: "Pip is cast as the teaspoon hero when a film crew starts shooting a movie in the kitchen.",
  episodeLabel: "S07E02",
  progressPercent: 42,
  seriesTitle: "Kettle Kids",
  timeLabel: "7:00–7:30 PM",
  title: "The Seventh Postcard",
} as const;

const missingArtworkEpisode = {
  ...classicEpisode,
  artworkState: "missing",
  badge: { label: "Up next", tone: "neutral" },
  progressPercent: undefined,
  timeLabel: "7:30–8:00 PM",
  title: "A Very Loud Doorbell at the Lighthouse",
} as const;

export { classicEpisode, missingArtworkEpisode };
