// @vitest-environment jsdom

import { act, cleanup, renderUi } from "@loomarr/test-support";
import type { ClientDestination } from "@loomarr/ui";
import { useEffect } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { TvScreens } from "../index";

afterEach(cleanup);

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
    const { container, rerender, unmount } = renderUi(null);
    const show = (active: ClientDestination) =>
      act(async () => rerender(<TvScreens active={active} overlay={<Overlay />} watching={<Watching />} />));
    await show("watching");
    // This package compiles without the DOM lib, so name the one node property read here.
    const text = () => (container as unknown as { textContent: string }).textContent;
    return { log, show, text, unmount: () => act(async () => unmount()) };
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
