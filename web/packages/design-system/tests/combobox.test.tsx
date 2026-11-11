// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Combobox, type ComboboxOption, type ComboboxProps, LoomarrProvider } from "../index";

// The web Combobox is Base UI's. axe cannot say whether the keys work, so each is pressed here: the
// text reaches the caller, the arrow keys move the highlight while focus stays in the input, Enter
// picks the highlighted option (and submits the form when none is highlighted), Escape closes.

afterEach(cleanup);

const places: readonly ComboboxOption[] = [
  { description: "Washington, United States", label: "Seattle", value: "seattle" },
  { description: "Oregon, United States", label: "Portland", trailing: "Added", value: "portland" },
  { disabled: true, label: "Spokane", value: "spokane" },
];

// A caller as the pickers are: it owns the text, and sets it to the label of what was picked.
const Harness = ({
  onPick = () => undefined,
  onType = () => undefined,
  ...props
}: Partial<ComboboxProps> & {
  onPick?: (option: ComboboxOption) => void;
  onType?: (text: string) => void;
}) => {
  const [text, setText] = useState("");
  return (
    <Combobox
      inputValue={text}
      label="Location"
      onInputValueChange={(next) => {
        onType(next);
        setText(next);
      }}
      onSelect={(option) => {
        onPick(option);
        setText(option.label);
      }}
      options={places}
      {...props}
    />
  );
};

const renderBox = (ui: React.ReactNode) => render(<LoomarrProvider>{ui}</LoomarrProvider>);

describe("Combobox (web)", () => {
  it("is a combobox named by its label, closed until the user types", async () => {
    renderBox(<Harness />);
    const input = screen.getByRole("combobox", { name: "Location" });

    expect(input.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("listbox")).toBeNull();

    await userEvent.type(input, "p");
    expect(input.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getAllByRole("option").map((option) => option.textContent)).toEqual([
      "SeattleWashington, United States",
      "PortlandOregon, United StatesAdded",
      "Spokane",
    ]);
  });

  it("hands the caller what is typed, and only that", async () => {
    const onType = vi.fn();
    renderBox(<Harness onType={onType} />);

    await userEvent.type(screen.getByRole("combobox"), "po");
    expect(onType.mock.calls.map(([text]) => text)).toEqual(["p", "po"]);

    await userEvent.click(await screen.findByRole("option", { name: /Portland/ }));
    // Picking writes the label into the input, but the caller hears it from `onSelect`, not as typing.
    expect(onType).toHaveBeenCalledTimes(2);
    expect((screen.getByRole("combobox") as HTMLInputElement).value).toBe("Portland");
  });

  it("walks the options with the arrow keys while focus stays in the input, and Enter picks one", async () => {
    const onPick = vi.fn();
    renderBox(<Harness onPick={onPick} />);
    const input = screen.getByRole("combobox");

    await userEvent.type(input, "s");
    await userEvent.keyboard("{ArrowDown}");
    const [seattle, portland] = screen.getAllByRole("option");
    expect(document.activeElement).toBe(input);
    expect(input.getAttribute("aria-activedescendant")).toBe(seattle?.id);

    await userEvent.keyboard("{ArrowDown}");
    expect(input.getAttribute("aria-activedescendant")).toBe(portland?.id);
    await userEvent.keyboard("{ArrowUp}{Enter}");

    expect(onPick).toHaveBeenCalledWith(expect.objectContaining({ value: "seattle" }));
    expect(document.activeElement).toBe(input);
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("opens from the arrow key alone and closes on Escape, keeping what was typed", async () => {
    renderBox(<Harness />);
    const input = screen.getByRole("combobox");

    await userEvent.type(input, "x");
    expect(screen.getByRole("listbox")).toBeTruthy();
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect((input as HTMLInputElement).value).toBe("x");

    await userEvent.keyboard("{ArrowDown}");
    expect(screen.getByRole("listbox")).toBeTruthy();
  });

  it("highlights the first option as the list changes when autoHighlight is on", async () => {
    const onPick = vi.fn();
    renderBox(<Harness autoHighlight onPick={onPick} />);

    await userEvent.type(screen.getByRole("combobox"), "s{Enter}");
    expect(onPick).toHaveBeenCalledWith(expect.objectContaining({ value: "seattle" }));
  });

  it("lets Enter submit the form when no option is highlighted", async () => {
    const onPick = vi.fn();
    const onSubmit = vi.fn((event: React.FormEvent) => event.preventDefault());
    renderBox(
      <form onSubmit={onSubmit}>
        <Harness onPick={onPick} />
        <button type="submit">Search</button>
      </form>,
    );

    await userEvent.type(screen.getByRole("combobox"), "sea{Enter}");
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(onPick).not.toHaveBeenCalled();
  });

  it("skips a disabled option and marks the current choice", async () => {
    const onPick = vi.fn();
    renderBox(<Harness onPick={onPick} value="portland" />);

    await userEvent.type(screen.getByRole("combobox"), "p");
    expect(screen.getByRole("option", { name: /Portland/ }).getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("option", { name: /Spokane/ }).getAttribute("aria-disabled")).toBe("true");

    await userEvent.click(screen.getByRole("option", { name: /Spokane/ }));
    expect(onPick).not.toHaveBeenCalled();
  });

  it("says when it is searching, found nothing or failed, and fades options from an earlier search", async () => {
    const searching = renderBox(<Harness loading options={[]} />);
    await userEvent.type(screen.getByRole("combobox"), "p");
    expect((await screen.findByRole("status")).textContent).toBe("Searching…");
    searching.unmount();

    const empty = renderBox(<Harness emptyText="No locations found." options={[]} />);
    await userEvent.type(screen.getByRole("combobox"), "p");
    expect((await screen.findByRole("status")).textContent).toBe("No locations found.");
    empty.unmount();

    const failed = renderBox(<Harness errorText="Location search is unavailable." options={[]} />);
    await userEvent.type(screen.getByRole("combobox"), "p");
    expect((await screen.findByRole("alert")).textContent).toBe("Location search is unavailable.");
    failed.unmount();

    const onPick = vi.fn();
    renderBox(<Harness onPick={onPick} stale />);
    await userEvent.type(screen.getByRole("combobox"), "p");
    await userEvent.click(screen.getByRole("option", { name: /Seattle/ }));
    expect(onPick).not.toHaveBeenCalled();
  });

  it("draws nothing when there is nothing to offer or say", async () => {
    renderBox(<Harness options={[]} />);

    await userEvent.type(screen.getByRole("combobox"), "p");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("takes its name from a visible label and reads that row's help with it", () => {
    renderBox(
      <>
        <span id="where">Where do you live?</span>
        <span id="where-help">City or country.</span>
        <Harness aria-describedby="where-help" aria-labelledby="where" invalid />
      </>,
    );
    const input = screen.getByRole("combobox", { name: "Where do you live?" });

    expect(input.getAttribute("aria-describedby")).toBe("where-help");
    expect(input.getAttribute("aria-invalid")).toBe("true");
  });

  it("cannot be used while disabled", async () => {
    renderBox(<Harness disabled />);
    const input = screen.getByRole<HTMLInputElement>("combobox");

    expect(input.disabled).toBe(true);
    await userEvent.type(input, "p");
    expect(screen.queryByRole("listbox")).toBeNull();
  });
});
