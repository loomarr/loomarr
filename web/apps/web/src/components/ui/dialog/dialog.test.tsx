import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "./dialog";

// The app's modal primitive. What is worth pinning is the behaviour the wrapper exists to inherit
// — the reason a hand-rolled `fixed inset-0` div is not good enough: the dialog is absent until
// opened, is announced as a modal with an accessible NAME and DESCRIPTION, closes on Escape, and
// closes from the injected corner button.
//
// ⚠ `findBy`, not `getBy`, throughout: Base UI mounts the portalled popup asynchronously, where
// Radix had it in the DOM by the time `click()` resolved. Same difference bit the menu tests.
const Confirm = ({ onOpenChange = () => {} }: { onOpenChange?: (open: boolean) => void }) => (
  <Dialog onOpenChange={onOpenChange}>
    <DialogTrigger>Edit job</DialogTrigger>
    <DialogContent>
      <DialogHeader>
        <DialogTitle>Edit job</DialogTitle>
        <DialogDescription>Change the schedule for this task.</DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <DialogClose>Cancel</DialogClose>
      </DialogFooter>
    </DialogContent>
  </Dialog>
);

describe("Dialog", () => {
  it("renders nothing until the trigger is activated", () => {
    render(<Confirm />);

    expect(screen.getByRole("button", { name: "Edit job" })).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("opens as a modal named and described by its own header", async () => {
    render(<Confirm />);
    await userEvent.click(screen.getByRole("button", { name: "Edit job" }));

    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveAccessibleName("Edit job");
    expect(dialog).toHaveAccessibleDescription("Change the schedule for this task.");
  });

  it("closes on Escape", async () => {
    const onOpenChange = vi.fn();
    render(<Confirm onOpenChange={onOpenChange} />);
    await userEvent.click(screen.getByRole("button", { name: "Edit job" }));
    await screen.findByRole("dialog");

    await userEvent.keyboard("{Escape}");
    expect(onOpenChange).toHaveBeenLastCalledWith(false, expect.anything());
  });

  it("closes from the injected corner button", async () => {
    const onOpenChange = vi.fn();
    render(<Confirm onOpenChange={onOpenChange} />);
    await userEvent.click(screen.getByRole("button", { name: "Edit job" }));

    await userEvent.click(await screen.findByRole("button", { name: "Close" }));
    expect(onOpenChange).toHaveBeenLastCalledWith(false, expect.anything());
  });
});
