// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Pressable, Text } from "react-native";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  LoomarrProvider,
} from "../index";

// The native Dialog (dialog.tsx), on React Native's Modal. Its triggers clone `onPress`, so they
// are Pressables rather than DOM buttons.

afterEach(cleanup);

const button = (label: string) => (
  <Pressable accessibilityRole="button">
    <Text>{label}</Text>
  </Pressable>
);

const renderDialog = (props: { dismissible?: boolean; onOpenChange?: (open: boolean) => void }) =>
  render(
    <LoomarrProvider>
      <Dialog {...props}>
        <DialogTrigger>{button("Edit job")}</DialogTrigger>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit job</DialogTitle>
            <DialogDescription>Change the schedule for this task.</DialogDescription>
          </DialogHeader>
          <DialogClose>{button("Done")}</DialogClose>
        </DialogContent>
      </Dialog>
    </LoomarrProvider>,
  );

describe("Dialog (native)", () => {
  it("draws nothing until the trigger is pressed, then its title and description", async () => {
    const onOpenChange = vi.fn();
    renderDialog({ onOpenChange });
    expect(screen.queryByText("Change the schedule for this task.")).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "Edit job" }));

    expect(screen.getByText("Change the schedule for this task.")).toBeTruthy();
    expect(onOpenChange).toHaveBeenLastCalledWith(true);
  });

  it.each(["Close", "Done"])("closes from the %s button", async (name) => {
    const onOpenChange = vi.fn();
    renderDialog({ onOpenChange });
    await userEvent.click(screen.getByRole("button", { name: "Edit job" }));

    await userEvent.click(screen.getByRole("button", { name }));

    expect(onOpenChange).toHaveBeenLastCalledWith(false);
    await waitFor(() => expect(screen.queryByText("Change the schedule for this task.")).toBeNull());
  });

  it("offers no corner close when it must be answered", async () => {
    renderDialog({ dismissible: false });
    await userEvent.click(screen.getByRole("button", { name: "Edit job" }));

    expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
    expect(screen.getByRole("button", { name: "Done" })).toBeTruthy();
  });
});
