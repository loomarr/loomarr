const classicEpisode = {
  artworkState: "ready",
  badge: { label: "On now", tone: "live" },
  channelName: "Classic Animation",
  channelNumber: "07",
  description: "Milhouse is cast as Fallout Boy when a Radioactive Man movie begins filming in Springfield.",
  episodeLabel: "S07E02",
  progressPercent: 42,
  seriesTitle: "The Simpsons",
  timeLabel: "7:00–7:30 PM",
  title: "Radioactive Man",
} as const;

const missingArtworkEpisode = {
  ...classicEpisode,
  artworkState: "missing",
  badge: { label: "Up next", tone: "neutral" },
  progressPercent: undefined,
  timeLabel: "7:30–8:00 PM",
  title: "Home Sweet Homediddly-Dum-Doodily",
} as const;

export { classicEpisode, missingArtworkEpisode };
