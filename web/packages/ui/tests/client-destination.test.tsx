// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it } from "vitest";

import { type ClientDestination, useClientDestination } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// A hardware Back press as the platform delivers it: the newest listener that answers true wins.
const fakeBackHandler = () => {
  const listeners: (() => boolean | undefined)[] = [];
  return {
    addEventListener: (_type: "hardwareBackPress", listener: () => boolean | undefined) => {
      listeners.push(listener);
      return { remove: () => void listeners.splice(listeners.indexOf(listener), 1) };
    },
    listeners,
    press: () => [...listeners].reverse().some((listener) => listener() === true),
  };
};

const mount = async (initial: ClientDestination, backHandler = fakeBackHandler()) => {
  let state!: ReturnType<typeof useClientDestination>;
  const Probe = () => {
    state = useClientDestination(initial, backHandler);
    return null;
  };
  const container = (
    globalThis as unknown as { document: { createElement: (tag: string) => never } }
  ).document.createElement("div");
  const root = createRoot(container);
  await act(async () => root.render(<Probe />));
  return {
    active: () => state[0],
    backHandler,
    go: (destination: ClientDestination) => act(async () => state[1](destination)),
    press: () => act(async () => void backHandler.press()),
    unmount: () => act(async () => root.unmount()),
  };
};

describe("useClientDestination", () => {
  it("starts on the destination it is given: the phone opens on its Guide", async () => {
    expect((await mount("guide")).active()).toBe("guide");
    expect((await mount("watching")).active()).toBe("watching");
  });

  it("takes Back from a transient screen to Watching and consumes the press", async () => {
    const shell = await mount("guide");
    let consumed = false;
    await act(async () => {
      consumed = shell.backHandler.press();
    });

    expect(consumed).toBe(true);
    expect(shell.active()).toBe("watching");
    await shell.unmount();
  });

  it("leaves Back from Watching to the platform, so the app goes home", async () => {
    const shell = await mount("guide");
    await shell.go("watching");

    let consumed = true;
    await act(async () => {
      consumed = shell.backHandler.press();
    });

    expect(consumed).toBe(false);
    expect(shell.active()).toBe("watching");
    await shell.unmount();
  });

  it("answers Back for the screen showing now, not the one it mounted on", async () => {
    const shell = await mount("watching");
    await shell.go("requests");
    await shell.press();
    expect(shell.active()).toBe("watching");
    await shell.unmount();
  });

  it("stops listening for Back when the shell goes away", async () => {
    const shell = await mount("guide");
    expect(shell.backHandler.listeners).toHaveLength(1);
    await shell.unmount();
    expect(shell.backHandler.listeners).toHaveLength(0);
  });
});
