// @vitest-environment jsdom

import { createGuideController } from "@loomarr/core/guide";
import { act, cleanup, renderUi } from "@loomarr/test-support";
import { afterEach, describe, expect, it, vi } from "vitest";

import { GuideJourney } from "../guide";

afterEach(cleanup);

const channel = (channelId: string, number: number) => ({
  airings: [
    {
      kind: "program" as const,
      scheduleBlockId: `${channelId}-block`,
      startMs: 0,
      stopMs: 3_600_000,
      title: `Show on ${channelId}`,
    },
  ],
  channelId,
  name: channelId,
  number,
  pendingCount: 0,
  status: "live" as const,
});

describe("GuideJourney on the phone", () => {
  it("re-picks the selection when the Favorites filter hides its channel", async () => {
    const controller = createGuideController({
      now: () => 900_000,
      source: {
        load: vi.fn().mockResolvedValue({
          channels: [channel("four", 4), channel("seven", 7)],
          fromMs: 0,
          timezone: "UTC",
          toMs: 3_600_000,
        }),
      },
    });
    await controller.refresh("four");
    expect(controller.getSnapshot().selection?.channelId).toBe("four");

    const { container } = renderUi(
      <GuideJourney
        controller={controller}
        density="touch"
        myChannels={{ favouriteIds: ["seven"], recentIds: [] }}
        onTune={vi.fn()}
      />,
    );
    await act(async () => {});
    // The selection stays while All is chosen.
    expect(controller.getSnapshot().selection?.channelId).toBe("four");

    act(() => container.querySelector<HTMLElement>('[aria-label^="Favorites"]')?.click());
    expect(controller.getSnapshot().selection?.channelId).toBe("seven");
  });
});
