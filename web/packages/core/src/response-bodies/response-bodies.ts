// The schema of what each device port reads, derived from the generated contract. Ports load this
// module with `import()` on their first read: the generated schemas are large, and the web app
// imports the port modules for their other exports, so a static import would put the schemas in
// its first download.

import { DevicePairPollResponse, DevicePairStartResponse, MeResponse } from "@loomarr/api/zod/auth";
import {
  ChannelGuideResponse,
  ChannelSubtitlesResponse,
  ChannelTracksResponse,
  GetChannelResponse,
  MyChannelsResponse,
} from "@loomarr/api/zod/channels";
import { SystemVersionResponse } from "@loomarr/api/zod/system";
import { z } from "zod";

// openEnum accepts any string for a field the contract lists as an enum, keeping the listed type,
// so an older client keeps working when a newer server adds a value.
const openEnum = <S extends z.ZodType>(_listed: S) =>
  z.custom<z.output<S>>((value) => typeof value === "string");

// The screens read most of the Guide, so the whole contract is parsed, with its enums open.
const timeline = ChannelGuideResponse.shape.channels.element;
const airing = timeline.shape.airings.element;
const pod = airing.shape.pod.unwrap();
const podEntry = pod.shape.entries.element;
const GuideBody = ChannelGuideResponse.omit({ $schema: true }).extend({
  channels: z.array(
    timeline.extend({
      status: openEnum(timeline.shape.status),
      airings: z.array(
        airing.extend({
          kind: openEnum(airing.shape.kind),
          pod: pod
            .extend({
              matchLevel: openEnum(pod.shape.matchLevel),
              entries: z.array(
                podEntry.extend({
                  role: openEnum(podEntry.shape.role),
                  audience: openEnum(podEntry.shape.audience.unwrap()).optional(),
                }),
              ),
            })
            .optional(),
        }),
      ),
    }),
  ),
});

// A role this client does not know reads as itself, which is not "admin"; the server decides access.
const AccountBody = MeResponse.pick({ name: true }).extend({ role: openEnum(MeResponse.shape.role) });

const MyChannelsBody = z.object({
  favourites: z.array(MyChannelsResponse.shape.favourites.element.pick({ channelId: true })),
  recent: z.array(MyChannelsResponse.shape.recent.element.pick({ channelId: true })),
});

const SubtitlesBody = ChannelSubtitlesResponse.pick({ cues: true, pending: true, tracks: true });

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
