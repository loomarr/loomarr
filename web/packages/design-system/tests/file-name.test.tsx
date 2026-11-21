// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { FileName, LoomarrProvider } from "../index";

// A file name gives way in the middle: the tail (the end of the name and its extension) always
// shows, the head takes what room is left, and the whole name is read once by assistive tech and
// shown on hover. Names here are invented.

afterEach(cleanup);

const parts = (name: string) => {
  const { container } = render(
    <LoomarrProvider>
      <FileName name={name} />
    </LoomarrProvider>,
  );
  return {
    container,
    head: container.querySelector('[data-part="head"]')?.textContent ?? null,
    tail: container.querySelector('[data-part="tail"]')?.textContent ?? null,
  };
};

describe("FileName", () => {
  it("keeps the last words and the extension of a long name, breaking at a word", () => {
    const { head, tail } = parts("Ashgrove.Station.1974.Remastered.1080p.mkv");
    expect(tail).toBe("Remastered.1080p.mkv");
    expect(head).toBe("Ashgrove.Station.1974.");
  });

  it("starts the tail at a word after a space, underscore or hyphen too", () => {
    expect(parts("Beacon Moor - Directors Cut - 1080p.mp4").tail).toBe("Cut - 1080p.mp4");
    cleanup();
    expect(parts("cinder_and_vale_s01e04_extended.m2ts").tail).toBe("s01e04_extended.m2ts");
  });

  it("never shortens a short name", () => {
    const { head, tail } = parts("intro.mkv");
    expect(head).toBeNull();
    expect(tail).toBe("intro.mkv");
  });

  it("keeps the end of a name with no extension", () => {
    const { head, tail } = parts("Brass Orchard Behind The Scenes Reel");
    expect(head).toBe("Brass Orchard Behind ");
    expect(tail).toBe("The Scenes Reel");
  });

  it("keeps the last eight characters of a name with no separator near its end", () => {
    const { head, tail } = parts("ashgrovestationtheextendedcut.mkv");
    expect(head).toBe("ashgrovestationtheext");
    expect(tail).toBe("endedcut.mkv");
  });

  it("does not read a leading dot or a long run after a dot as an extension", () => {
    expect(parts(".cartographer-daughter-notes").tail).toBe("daughter-notes");
    cleanup();
    expect(parts("Calder Pier Holiday.Footage").tail).toBe("Holiday.Footage");
  });

  it("gives assistive tech the whole name once and the pointer a title", () => {
    const name = "Ashgrove.Station.1974.Remastered.1080p.mkv";
    const { container } = parts(name);
    expect(screen.getByText(name)).toBeTruthy();
    for (const drawn of container.querySelectorAll("[data-part]")) {
      expect(drawn.getAttribute("aria-hidden")).toBe("true");
    }
    expect(container.querySelector(`[title="${name}"]`)).toBeTruthy();
  });

  it("sets the name on one line in the data face", () => {
    const { container } = parts("Ashgrove.Station.1974.Remastered.1080p.mkv");
    const head = container.querySelector<HTMLElement>('[data-part="head"]');
    const tail = container.querySelector<HTMLElement>('[data-part="tail"]');
    expect(head?.style.whiteSpace).toBe("pre");
    expect(head?.style.textOverflow).toBe("ellipsis");
    expect(tail?.style.whiteSpace).toBe("pre");
    expect(tail?.style.flexShrink).toBe("0");
    expect(container.innerHTML).toMatch(/Geist Mono/);
  });
});
