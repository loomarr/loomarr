import type { ChannelPlaySource } from "../channel-play-url";

type WarmedChannel = ChannelPlaySource & {
  warmed: boolean;
};

export type { WarmedChannel };
