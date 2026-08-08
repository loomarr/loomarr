import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Volume2 } from "lucide-react";
import { describe, expect, it, vi } from "vitest";
import { TrackSelectMenu } from "./track-select-menu";

const OPTS = [
  { value: "eng", label: "English" },
  { value: "spa", label: "Spanish" },
];

describe("TrackSelectMenu", () => {
  // The trigger is ICON-ONLY (like FullscreenButton) — it does NOT print the current track's label,
  // it just carries the accessible name ("Audio"). Which track is current shows as a check INSIDE the
  // open menu, so that is where the "user can tell what's selected" guarantee is pinned.
  it("keeps the trigger icon-only and marks the current track checked in the menu", async () => {
    render(<TrackSelectMenu icon={Volume2} label="Audio" options={OPTS} value="spa" onChange={() => {}} />);

    // No label text bleeds onto the trigger — its accessible name is exactly "Audio".
    const trigger = screen.getByRole("button", { name: "Audio" });
    expect(trigger).not.toHaveTextContent("Spanish");

    // The current selection is legible once opened: Spanish is checked, English is not.
    await userEvent.click(trigger);
    expect(screen.getByRole("menuitemcheckbox", { name: "Spanish" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("menuitemcheckbox", { name: "English" })).toHaveAttribute(
      "aria-checked",
      "false",
    );
  });

  it("opens and picks an option", async () => {
    const onChange = vi.fn();
    render(<TrackSelectMenu icon={Volume2} label="Audio" options={OPTS} value="eng" onChange={onChange} />);
    await userEvent.click(screen.getByRole("button", { name: "Audio" }));
    // Radix renders the items as menuitemcheckbox; pick Spanish.
    await userEvent.click(screen.getByRole("menuitemcheckbox", { name: "Spanish" }));
    expect(onChange).toHaveBeenCalledWith("spa");
  });

  it("disables the options for a read-only member", async () => {
    const onChange = vi.fn();
    render(
      <TrackSelectMenu
        icon={Volume2}
        label="Audio"
        options={OPTS}
        value="eng"
        onChange={onChange}
        readOnly
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Audio" }));
    const spanish = screen.getByRole("menuitemcheckbox", { name: "Spanish" });
    expect(spanish).toHaveAttribute("data-disabled");
  });
});
