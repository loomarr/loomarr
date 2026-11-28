// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Action, Badge, Chip, LoomarrProvider, Text } from "../index";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const alignOf = (element: Element) => getComputedStyle(element).alignSelf;

describe("alignSelf on the primitives", () => {
  it("places an Action on its Pressable host and on a `render` host", () => {
    render(
      <LoomarrProvider theme="dark">
        <Action alignSelf="flex-end">Save</Action>
        <Action alignSelf="center" render={<a href="/guide">Guide</a>}>
          Guide
        </Action>
      </LoomarrProvider>,
    );

    expect(alignOf(screen.getByRole("button", { name: "Save" }))).toBe("flex-end");
    expect(alignOf(screen.getByRole("button", { name: "Guide" }))).toBe("center");
  });

  it("places a submit Action's native button", () => {
    render(
      <LoomarrProvider theme="dark">
        <Action alignSelf="flex-start" submit>
          Continue
        </Action>
      </LoomarrProvider>,
    );

    expect(alignOf(screen.getByRole("button", { name: "Continue" }))).toBe("flex-start");
  });

  it("places a Text on the Tamagui host, an `as` host and a haloed wrapper", () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => undefined);
    render(
      <LoomarrProvider theme="dark">
        <Text alignSelf="center" data-testid="tamagui" textRole="body">
          A title
        </Text>
        <Text alignSelf="center" as="span" data-testid="host" textRole="body">
          A title
        </Text>
        <div data-testid="row" style={{ display: "flex" }}>
          <Text alignSelf="center" halo textRole="metadata">
            TUNING IN
          </Text>
        </div>
      </LoomarrProvider>,
    );

    expect(alignOf(screen.getByTestId("tamagui"))).toBe("center");
    expect(alignOf(screen.getByTestId("host"))).toBe("center");
    expect(screen.getByTestId("host").hasAttribute("alignself")).toBe(false);
    expect(alignOf(screen.getByTestId("row").firstElementChild as Element)).toBe("center");
    expect(error).not.toHaveBeenCalled();
  });

  it("moves a Badge of either shape, and leaves a pill at the start when unset", () => {
    render(
      <LoomarrProvider theme="dark">
        <Badge data-testid="pill" tone="info">
          Live
        </Badge>
        <Badge alignSelf="center" data-testid="centred-pill" tone="info">
          Live
        </Badge>
        <Badge alignSelf="flex-end" data-testid="square" shape="square">
          4K
        </Badge>
      </LoomarrProvider>,
    );

    expect(alignOf(screen.getByTestId("pill"))).toBe("flex-start");
    expect(alignOf(screen.getByTestId("centred-pill"))).toBe("center");
    expect(alignOf(screen.getByTestId("square"))).toBe("flex-end");
  });

  it("places a Chip's pressable", () => {
    render(
      <LoomarrProvider theme="dark">
        <Chip alignSelf="flex-start">2H</Chip>
      </LoomarrProvider>,
    );

    expect(alignOf(screen.getByRole("button", { name: "2H" }))).toBe("flex-start");
  });

  it("sets no alignment on an Action that was given none", () => {
    render(
      <LoomarrProvider theme="dark">
        <Action render={<a href="/guide">Guide</a>}>Guide</Action>
      </LoomarrProvider>,
    );

    expect(screen.getByRole("button", { name: "Guide" }).style.alignSelf).toBe("");
  });
});
