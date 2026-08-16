import type { ChannelTemplate } from "./templates.type";

// The starter intents §13 names by hand — product data, not fixtures: they ship in
// the FE bundle and a user edits one before running it. They live in packages/core so
// the wizard's guided first channel (13.3) and the Suggest workspace (13.4) offer the
// same set, and mobile inherits it. Each is phrased the way a person actually talks,
// because the suggester grounds on the sentence, not on the label.
const CHANNEL_TEMPLATES: ChannelTemplate[] = [
  {
    id: "saturday-cartoons",
    label: "90s Saturday Morning Cartoons",
    intent: {
      description: "Saturday-morning cartoons like I watched as a kid — bright, silly, kid-safe",
      era: "1990s",
      tone: "playful",
    },
  },
  {
    id: "cozy-mystery",
    label: "Cozy Mystery Nights",
    intent: {
      description: "Gentle small-town mysteries for a rainy evening — nothing gruesome",
      tone: "cozy",
    },
  },
  {
    id: "late-night-scifi",
    label: "Late-Night Sci-Fi",
    intent: {
      description: "Weird, atmospheric science fiction for after midnight",
      tone: "moody",
    },
  },
  {
    id: "action-marathon",
    label: "Action Movie Marathon",
    intent: {
      description: "Back-to-back action movies, high energy, keep it PG-13",
      tone: "high energy",
    },
  },
];

export { CHANNEL_TEMPLATES };
