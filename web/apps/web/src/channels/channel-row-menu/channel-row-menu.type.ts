import type { ChannelDTO } from "@loomarr/api/models/channelDTO";

type ChannelRowMenu = {
  channel: Pick<ChannelDTO, "id" | "name" | "status">;
};

export type { ChannelRowMenu };
