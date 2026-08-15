import type { ChannelDTO } from "@loomarr/api";

type ChannelRowMenu = {
  channel: Pick<ChannelDTO, "id" | "name" | "status">;
};

export type { ChannelRowMenu };
