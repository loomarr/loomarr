// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "../index";

// The web Dialog, on Base UI. What it owes a keyboard user is pinned here by driving the keys, not
// by reading attributes: focus enters on open, Tab cannot leave, Escape closes, and focus goes back
// to whatever opened it. The native adapter is a React Native Modal and is not exercised here:
// react-native-web's Modal would stand in for it and hide exactly the focus bugs a device shows.

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const EditJob = ({ onOpenChange }: { onOpenChange?: (open: boolean) => void }) => (
  <>
    <button type="button">Before</button>
    <Dialog onOpenChange={onOpenChange}>
      <DialogTrigger>
        <button type="button">Edit job</button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit job</DialogTitle>
          <DialogDescription>Change the schedule for this task.</DialogDescription>
        </DialogHeader>
        <label>
          Schedule
          <input name="schedule" />
        </label>
        <DialogFooter>
          <DialogClose>
            <button type="button">Cancel</button>
          </DialogClose>
          <button type="button">Save</button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
    <button type="button">After</button>
  </>
);

// A dialog a page opens from its own state, with no trigger part: the tasks page's job editor.
const Controlled = () => {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Modify
      </button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogTitle>Modify task</DialogTitle>
          <input aria-label="Cron" />
        </DialogContent>
      </Dialog>
    </>
  );
};

type User = ReturnType<typeof userEvent.setup>;

// Base UI bounds the trap with two invisible focus guards: Tab onto one, and on the next frame it
// hands focus to the far end of the dialog. A press waits for that hand-off, as a person would.
const press = async (user: User, shift = false) => {
  await user.tab({ shift });
  await waitFor(() => expect(document.activeElement?.hasAttribute("data-base-ui-focus-guard")).toBe(false));
};

const focused = () =>
  document.activeElement?.getAttribute("aria-label") ?? document.activeElement?.textContent;

const openWithKeyboard = async (user: User, trigger: string) => {
  await user.tab();
  while (document.activeElement !== screen.getByRole("button", { name: trigger })) await user.tab();
  await user.keyboard("{Enter}");
  return screen.findByRole("dialog");
};

describe("Dialog", () => {
  it("renders nothing until the trigger is activated", () => {
    render(<EditJob />);

    expect(screen.getByRole("button", { name: "Edit job" })).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("opens as a modal named and described by its own header", async () => {
    const user = userEvent.setup();
    render(<EditJob />);
    await user.click(screen.getByRole("button", { name: "Edit job" }));

    await screen.findByRole("dialog", {
      name: "Edit job",
      description: "Change the schedule for this task.",
    });
    // Modal: the page behind leaves the accessibility tree.
    expect(screen.queryByRole("button", { name: "Before" })).toBeNull();
    expect(screen.queryByRole("button", { name: "After" })).toBeNull();
  });

  it("moves focus into the dialog when the keyboard opens it", async () => {
    const user = userEvent.setup();
    render(<EditJob />);
    const dialog = await openWithKeyboard(user, "Edit job");

    expect(dialog.contains(document.activeElement)).toBe(true);
    expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Schedule" }));
  });

  it("keeps Tab and Shift+Tab inside the dialog", async () => {
    const user = userEvent.setup();
    render(<EditJob />);
    const dialog = await openWithKeyboard(user, "Edit job");

    const forward: (string | null | undefined)[] = [];
    for (let step = 0; step < 4; step += 1) {
      await press(user);
      forward.push(focused());
      expect(dialog.contains(document.activeElement), String(forward)).toBe(true);
    }
    // Past the corner Close, Tab comes round to the field instead of leaving for the page.
    expect(forward).toEqual(["Cancel", "Save", "Close", ""]);
    expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Schedule" }));

    await press(user, true);
    expect(focused()).toBe("Close");
  });

  it("closes on Escape and returns focus to the trigger", async () => {
    const onOpenChange = vi.fn();
    const user = userEvent.setup();
    render(<EditJob onOpenChange={onOpenChange} />);
    await openWithKeyboard(user, "Edit job");

    await user.keyboard("{Escape}");

    expect(onOpenChange).toHaveBeenLastCalledWith(false);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Edit job" }));
  });

  it("closes from the corner button and from a caller's close action", async () => {
    const onOpenChange = vi.fn();
    const user = userEvent.setup();
    render(<EditJob onOpenChange={onOpenChange} />);

    await user.click(screen.getByRole("button", { name: "Edit job" }));
    await user.click(await screen.findByRole("button", { name: "Close" }));
    expect(onOpenChange).toHaveBeenLastCalledWith(false);
    expect(screen.queryByRole("dialog")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Edit job" }));
    await user.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(onOpenChange).toHaveBeenCalledTimes(4);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("returns focus to the control that opened a dialog with no trigger", async () => {
    const user = userEvent.setup();
    render(<Controlled />);
    const modify = screen.getByRole("button", { name: "Modify" });
    await user.tab();
    expect(document.activeElement).toBe(modify);
    await user.keyboard("{Enter}");

    await screen.findByRole("dialog", { name: "Modify task" });
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Cron" })));

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(modify);
  });

  // jsdom never matches `:focus-visible`, so it stands in for the browser here: focus that arrives by
  // the keyboard is visible, the way Chrome, Firefox and Safari decide it.
  it("rings the corner button while the browser marks its focus visible", async () => {
    const user = userEvent.setup();
    render(<EditJob />);
    await user.click(screen.getByRole("button", { name: "Edit job" }));
    const close = await screen.findByRole("button", { name: "Close" });
    const matches = HTMLElement.prototype.matches;
    vi.spyOn(close, "matches").mockImplementation((selector) =>
      selector === ":focus-visible" ? close === document.activeElement : matches.call(close, selector),
    );

    expect(close.style.boxShadow).toBe("");
    while (document.activeElement !== close) await press(user);
    expect(close.style.boxShadow).toBe("0 0 0 2px #FFB020");

    await press(user);
    expect(close.style.boxShadow).toBe("");
  });
});
