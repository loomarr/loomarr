// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

// A caller returns focus to an Action through a ref (#4476), including one a Menu draws as its
// trigger. The second run takes the native path, where the Action draws a Pressable.
const platform = vi.hoisted(() => ({ web: true }));
vi.mock("@tamagui/core", async (importOriginal) => {
  const real = await importOriginal<typeof import("@tamagui/core")>();
  return {
    ...real,
    get isWeb() {
      return platform.web;
    },
  };
});

import { Action, LoomarrProvider, Menu, MenuAction, MenuContent, MenuTrigger } from "../index";

afterEach(() => {
  cleanup();
  platform.web = true;
});

describe("Action ref", () => {
  it("focuses an Action that is a menu's trigger, and keeps the menu working", async () => {
    const ref = createRef<{ focus: () => void }>();
    render(
      <LoomarrProvider>
        <Menu>
          <MenuTrigger>
            <Action ref={ref as never}>Fix</Action>
          </MenuTrigger>
          <MenuContent>
            <MenuAction onSelect={vi.fn()}>Update</MenuAction>
          </MenuContent>
        </Menu>
      </LoomarrProvider>,
    );
    const trigger = screen.getByRole("button", { name: "Fix" });
    ref.current?.focus();
    expect(document.activeElement).toBe(trigger);
    await userEvent.click(trigger);
    await waitFor(() => expect(screen.getByRole("menu")).toBeTruthy());
  });

  it("reaches the pressable on native", () => {
    platform.web = false;
    const ref = createRef<{ focus: () => void }>();
    render(
      <LoomarrProvider>
        <Action ref={ref as never}>Fix</Action>
      </LoomarrProvider>,
    );
    ref.current?.focus();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Fix" }));
  });
});
