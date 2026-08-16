import channelTemplates from "./channel-templates.json";
import type { ChannelTemplate } from "./templates.type";

// The starter intents §13 names by hand — product data, not fixtures: they ship in
// the FE bundle and a user edits one before running it. They live in packages/core so
// the wizard's guided first channel (13.3) and the Suggest workspace (13.4) offer the
// same set, and mobile inherits it. Each is phrased the way a person actually talks,
// because the suggester grounds on the sentence, not on the label.
const CHANNEL_TEMPLATES: ChannelTemplate[] = channelTemplates;

export { CHANNEL_TEMPLATES };
