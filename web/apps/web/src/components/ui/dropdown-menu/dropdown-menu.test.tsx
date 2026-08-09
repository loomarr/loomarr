import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "./dropdown-menu";

// The app's menu primitive (a thin themed wrapper over Base UI Menu). What is worth pinning is that
// the wrapper preserves the behaviour it exists to theme: the content is closed until the trigger
// opens it (and is portalled), a CheckboxItem carries its checked state as accessible ARIA, and a
// chosen item fires onCheckedChange. The animation classes are visual-baseline territory, not
// asserted here.
const Menu = ({ checked = false, onChange = () => {} }: { checked?: boolean; onChange?: () => void }) => (
  <DropdownMenu>
    <DropdownMenuTrigger>Open</DropdownMenuTrigger>
    <DropdownMenuContent>
      <DropdownMenuGroup>
        <DropdownMenuLabel>Audio</DropdownMenuLabel>
        <DropdownMenuCheckboxItem checked={checked} onCheckedChange={onChange}>
          English
        </DropdownMenuCheckboxItem>
        <DropdownMenuSeparator />
        <DropdownMenuCheckboxItem checked={!checked} onCheckedChange={onChange}>
          Spanish
        </DropdownMenuCheckboxItem>
      </DropdownMenuGroup>
    </DropdownMenuContent>
  </DropdownMenu>
);

describe("DropdownMenu", () => {
  it("stays closed until the trigger is activated", async () => {
    render(<Menu />);
    expect(screen.queryByRole("menuitemcheckbox", { name: "English" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    expect(await screen.findByRole("menuitemcheckbox", { name: "English" })).toBeInTheDocument();
  });

  it("reflects each item's checked state as ARIA", async () => {
    render(<Menu checked />);
    await userEvent.click(screen.getByRole("button", { name: "Open" }));

    expect(await screen.findByRole("menuitemcheckbox", { name: "English" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    expect(screen.getByRole("menuitemcheckbox", { name: "Spanish" })).toHaveAttribute(
      "aria-checked",
      "false",
    );
  });

  it("fires onCheckedChange when an item is chosen", async () => {
    const onChange = vi.fn();
    render(<Menu onChange={onChange} />);
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    await userEvent.click(await screen.findByRole("menuitemcheckbox", { name: "Spanish" }));

    expect(onChange).toHaveBeenCalledOnce();
  });
});
