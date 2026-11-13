// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LoomarrProvider, Menu, MenuAction, MenuContent, MenuGroup, MenuTrigger } from "../index";

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
    const menu = await screen.findByRole("menu");
    // The menu moves focus into itself after it opens; arrows pressed before then reach the
    // trigger, and the menu never sees them (#3567).
    await waitFor(() => expect(menu.contains(document.activeElement)).toBe(true));
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

  // An armed confirmation is a group of menu items named by its question, never a panel of
  // buttons: a menu may hold only items and groups, and only items take the arrow keys.
  it("names a group by its label and reaches its actions by keyboard", async () => {
    const onDelete = vi.fn();
    renderMenu(
      <Menu defaultOpen>
        <MenuTrigger>
          <button type="button">Actions</button>
        </MenuTrigger>
        <MenuContent>
          <MenuGroup label="Delete Morning Cartoons for good?" tone="danger">
            <MenuAction onSelect={onDelete} tone="danger">
              Delete
            </MenuAction>
            <MenuAction onSelect={vi.fn()}>Cancel</MenuAction>
          </MenuGroup>
        </MenuContent>
      </Menu>,
    );

    const group = await screen.findByRole("group", { name: "Delete Morning Cartoons for good?" });
    expect(
      within(group)
        .getAllByRole("menuitem")
        .map((item) => item.textContent),
    ).toEqual(["Delete", "Cancel"]);
    // A defaultOpen menu moves focus into itself a frame after mount; keys pressed before then
    // land on the body and the menu never sees them.
    await waitFor(() => expect(screen.getByRole("menu").contains(document.activeElement)).toBe(true));
    await userEvent.keyboard("{ArrowDown}{Enter}");
    expect(onDelete).toHaveBeenCalledOnce();
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
