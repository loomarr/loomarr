// The schema of what each device port reads, derived from the generated contract. Ports load this
// module with `import()` on their first read: the generated schemas are large, and the web app
// imports the port modules for their other exports, so a static import would put the schemas in
// its first download.

import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";
import type { SubtitleCueDTO } from "@loomarr/api/models/subtitleCueDTO";
import { DevicePairPollResponse, DevicePairStartResponse, MeResponse } from "@loomarr/api/zod/auth";
import {
  ChannelGuideResponse,
  ChannelSubtitlesResponse,
  ChannelTracksResponse,
  GetChannelResponse,
} from "@loomarr/api/zod/channels";
import { SystemVersionResponse } from "@loomarr/api/zod/system";
import { z } from "zod";

// openEnum accepts any string for a field the contract lists as an enum, keeping the listed type,
// so an older client keeps working when a newer server adds a value.
const openEnum = <S extends z.ZodType>(_listed: S) =>
  z.custom<z.output<S>>((value) => typeof value === "string");

// Long lists are checked for the fields the client's own logic reads, and each item passes through
// as the server sent it. Validating and copying every field of a large Guide costs more than
// parsing its JSON, and a television reads the Guide often.
type FieldTypes<T> = { [K in keyof T]?: "number" | "string" };

const itemCheck = <T>(fields: FieldTypes<T>) => {
  const required = Object.entries(fields) as [string, string][];
  return (value: unknown): value is T => {
    if (typeof value !== "object" || value === null) return false;
    for (const [name, type] of required) {
      if (typeof (value as Record<string, unknown>)[name] !== type) return false;
    }
    return true;
  };
};

const listOf = <T>(check: (value: unknown) => value is T) =>
  z.custom<T[]>((value) => Array.isArray(value) && value.every(check));

type GuideChannel = GuideOutputBody["channels"][number];
const isAiring = itemCheck<GuideChannel["airings"][number]>({
  kind: "string",
  scheduleBlockId: "string",
  startMs: "number",
  stopMs: "number",
  title: "string",
});
const hasChannelFields = itemCheck<GuideChannel>({
  channelId: "string",
  name: "string",
  number: "number",
  pendingCount: "number",
  status: "string",
});
const isChannel = (value: unknown): value is GuideChannel =>
  hasChannelFields(value) && Array.isArray(value.airings) && value.airings.every(isAiring);

const GuideBody = ChannelGuideResponse.pick({ fromMs: true, timezone: true, toMs: true }).extend({
  channels: listOf(isChannel),
});

// A role this client does not know reads as itself, which is not "admin"; the server decides access.
const AccountBody = MeResponse.pick({ name: true }).extend({ role: openEnum(MeResponse.shape.role) });

const listed = listOf(itemCheck<{ channelId: string }>({ channelId: "string" }));
const MyChannelsBody = z.object({ favourites: listed, recent: listed });

const isCue = itemCheck<SubtitleCueDTO>({ endMs: "number", startMs: "number", text: "string" });
const SubtitlesBody = ChannelSubtitlesResponse.pick({ pending: true, tracks: true }).extend({
  cues: listOf(isCue),
});

const TracksBody = ChannelTracksResponse.pick({ audio: true });
const playout = GetChannelResponse.shape.policy.shape.playout.unwrap();
const ChannelAudioBody = z.object({
  policy: z.object({ playout: playout.pick({ audioLanguage: true }).optional() }),
});

const VersionBody = SystemVersionResponse.pick({ dirty: true, version: true });

const PairStartBody = DevicePairStartResponse.pick({
  deviceCode: true,
  expiresAt: true,
  interval: true,
  userCode: true,
});
const PairPollBody = DevicePairPollResponse.pick({ deviceName: true, token: true });

export {
  AccountBody,
  ChannelAudioBody,
  GuideBody,
  MyChannelsBody,
  PairPollBody,
  PairStartBody,
  SubtitlesBody,
  TracksBody,
  VersionBody,
};
