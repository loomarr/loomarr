import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from "./sheet";

const Example = () => (
  <Sheet swipeDirection="right">
    <SheetTrigger>Manage Ada</SheetTrigger>
    <SheetContent>
      <SheetHeader>
        <SheetTitle>Ada Lovelace</SheetTitle>
        <SheetDescription>Manage access and sessions.</SheetDescription>
      </SheetHeader>
      <button type="button">Detail action</button>
    </SheetContent>
  </Sheet>
);

describe("Sheet", () => {
  it("opens as a named modal and closes on Escape", async () => {
    render(<Example />);
    const trigger = screen.getByRole("button", { name: "Manage Ada" });
    await userEvent.click(trigger);

    const sheet = await screen.findByRole("dialog", { name: "Ada Lovelace" });
    expect(sheet).toHaveAccessibleDescription("Manage access and sessions.");
    expect(sheet).toHaveFocus();

    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(trigger).toHaveFocus();
  });

  it("closes from its injected close control", async () => {
    render(<Example />);
    await userEvent.click(screen.getByRole("button", { name: "Manage Ada" }));
    await userEvent.click(await screen.findByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });
});
