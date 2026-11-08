// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LoomarrProvider, Menu, MenuAction, MenuContent, MenuTrigger } from "../index";

// The web Menu, on Base UI. It owes a browser user a real menu: the trigger opens it, the actions
// are menuitems reachable by keyboard, picking one runs it and closes the menu, and an action that
// arms a confirmation can keep it open. The native adapter refuses to render and is not exercised.

afterEach(cleanup);

const renderMenu = (ui: React.ReactNode) => render(<LoomarrProvider>{ui}</LoomarrProvider>);

describe("Menu (web)", () => {
  it("opens from its trigger, runs the picked action and closes", async () => {
    const onWatch = vi.fn();
    renderMenu(
      <Menu>
        <MenuTrigger>
          <button type="button">Actions for Bluey</button>
        </MenuTrigger>
        <MenuContent>
          <MenuAction onSelect={onWatch}>Watch</MenuAction>
          <MenuAction onSelect={vi.fn()}>Pause</MenuAction>
        </MenuContent>
      </Menu>,
    );

    await userEvent.click(screen.getByRole("button", { name: "Actions for Bluey" }));
    expect(await screen.findByRole("menu")).toBeTruthy();
    await userEvent.click(screen.getByRole("menuitem", { name: "Watch" }));

    expect(onWatch).toHaveBeenCalledOnce();
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  });

  it("is reachable by keyboard", async () => {
    const onPause = vi.fn();
    renderMenu(
      <Menu>
        <MenuTrigger>
          <button type="button">Actions</button>
        </MenuTrigger>
        <MenuContent>
          <MenuAction onSelect={vi.fn()}>Watch</MenuAction>
          <MenuAction onSelect={onPause}>Pause</MenuAction>
        </MenuContent>
      </Menu>,
    );

    screen.getByRole("button", { name: "Actions" }).focus();
    await userEvent.keyboard("{Enter}");
    await screen.findByRole("menu");
    await userEvent.keyboard("{ArrowDown}{ArrowDown}{Enter}");

    expect(onPause).toHaveBeenCalledOnce();
  });

  it("keeps the menu open for an action that arms a confirmation, and tells the caller when it closes", async () => {
    const onOpenChange = vi.fn();
    const Confirming = () => {
      const [armed, setArmed] = useState(false);
      return (
        <Menu onOpenChange={onOpenChange}>
          <MenuTrigger>
            <button type="button">Actions</button>
          </MenuTrigger>
          <MenuContent>
            {armed ? (
              <p>Delete for good?</p>
            ) : (
              <MenuAction closeOnSelect={false} onSelect={() => setArmed(true)} tone="danger">
                Delete…
              </MenuAction>
            )}
          </MenuContent>
        </Menu>
      );
    };
    renderMenu(<Confirming />);

    await userEvent.click(screen.getByRole("button", { name: "Actions" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Delete…" }));
    expect(screen.getByText("Delete for good?")).toBeTruthy();
    expect(onOpenChange).toHaveBeenLastCalledWith(true);

    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(onOpenChange).toHaveBeenLastCalledWith(false));
  });

  it("does not run a disabled action", async () => {
    const onPause = vi.fn();
    renderMenu(
      <Menu>
        <MenuTrigger>
          <button type="button">Actions</button>
        </MenuTrigger>
        <MenuContent>
          <MenuAction disabled onSelect={onPause}>
            Pause
          </MenuAction>
        </MenuContent>
      </Menu>,
    );

    await userEvent.click(screen.getByRole("button", { name: "Actions" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Pause" }));

    expect(onPause).not.toHaveBeenCalled();
  });
});
