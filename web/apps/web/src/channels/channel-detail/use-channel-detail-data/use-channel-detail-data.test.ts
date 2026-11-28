import * as channelsApi from "@loomarr/api/endpoints/channels";
import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { invalidateChannelDetail } from "./use-channel-detail-data";

describe("invalidateChannelDetail", () => {
  it("marks the channel's five reads stale and leaves another channel's alone", () => {
    const client = new QueryClient();
    const mine = [
      channelsApi.getGetChannelQueryKey("ch1"),
      channelsApi.getChannelsNowNextQueryKey(),
      channelsApi.getChannelUpcomingQueryKey("ch1"),
      channelsApi.getPreviewChannelPodsQueryKey("ch1"),
      channelsApi.getPreviewChannelCycleQueryKey("ch1"),
    ];
    const other = channelsApi.getGetChannelQueryKey("ch2");
    for (const key of [...mine, other]) client.setQueryData(key, {});

    invalidateChannelDetail(client, "ch1");

    for (const key of mine) expect(client.getQueryState(key)?.isInvalidated).toBe(true);
    expect(client.getQueryState(other)?.isInvalidated).toBe(false);
  });
});
