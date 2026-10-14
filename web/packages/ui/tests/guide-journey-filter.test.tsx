// @vitest-environment jsdom

import { createGuideController } from "@loomarr/core/guide";
import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { GuideJourney } from "../index";

(
  globalThis as typeof globalThis & {
    IS_REACT_ACT_ENVIRONMENT: boolean;
  }
).IS_REACT_ACT_ENVIRONMENT = true;

interface DomElement {
  click: () => void;
  querySelector: (selector: string) => DomElement | null;
}

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

    const container = (
      globalThis as unknown as { document: { createElement: (tagName: string) => DomElement } }
    ).document.createElement("div");
    const root = createRoot(container as unknown as Parameters<typeof createRoot>[0]);
    act(() =>
      root.render(
        <LoomarrProvider>
          <GuideJourney
            controller={controller}
            density="touch"
            myChannels={{ favouriteIds: ["seven"], recentIds: [] }}
            onTune={vi.fn()}
          />
        </LoomarrProvider>,
      ),
    );
    await act(async () => {});
    // The selection stays while All is chosen.
    expect(controller.getSnapshot().selection?.channelId).toBe("four");

    act(() => container.querySelector('[aria-label^="Favorites"]')?.click());
    expect(controller.getSnapshot().selection?.channelId).toBe("seven");
    act(() => root.unmount());
  });
});
