// @vitest-environment jsdom

import type { ClientDestination } from "@loomarr/ui";
import { act, useEffect } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it } from "vitest";

import { TvScreens } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

describe("TvScreens", () => {
  const mount = async () => {
    const log: string[] = [];
    const Watching = () => {
      useEffect(() => {
        log.push("watching mounted");
        return () => void log.push("watching unmounted");
      }, []);
      return <span>watching</span>;
    };
    const Overlay = () => {
      useEffect(() => {
        log.push("overlay mounted");
        return () => void log.push("overlay unmounted");
      }, []);
      return <span>overlay</span>;
    };
    const container = (
      globalThis as unknown as {
        document: { createElement: (tag: string) => never };
      }
    ).document.createElement("div") as unknown as { textContent: string };
    const root = createRoot(container as never);
    const show = (active: ClientDestination) =>
      act(async () =>
        root.render(<TvScreens active={active} overlay={<Overlay />} watching={<Watching />} />),
      );
    await show("watching");
    return { log, show, text: () => container.textContent, unmount: () => act(async () => root.unmount()) };
  };

  it("draws only Watching on Watching", async () => {
    const screens = await mount();
    expect(screens.text()).toBe("watching");
    expect(screens.log).toEqual(["watching mounted"]);
    await screens.unmount();
  });

  it("draws the Guide or Surf over Watching, which stays mounted beneath", async () => {
    const screens = await mount();

    await screens.show("guide");
    expect(screens.text()).toBe("watchingoverlay");
    await screens.show("surf");
    await screens.show("watching");

    // Watching mounted once for the whole visit; the overlay came and went with the screen.
    expect(screens.log).toEqual(["watching mounted", "overlay mounted", "overlay unmounted"]);
    await screens.unmount();
  });
});
