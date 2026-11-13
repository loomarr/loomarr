// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ComponentProps, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  Combobox,
  type ComboboxGroup,
  type ComboboxOption,
  type ComboboxProps,
  LoomarrProvider,
} from "../index";

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

const palette: readonly ComboboxGroup[] = [
  {
    id: "channels",
    label: "Channels",
    options: [{ label: "90s Action", trailing: "#42", value: "c1" }],
  },
  { id: "none", label: "Nothing here", options: [] },
  {
    id: "library",
    label: "In your library",
    options: [
      { label: "The Velvet Cartographer", trailing: <b>In library</b>, value: "l1" },
      { label: "Pelican Point Motel", value: "l2" },
    ],
  },
];

const activeOption = () => {
  const id = screen.getByRole("combobox").getAttribute("aria-activedescendant");
  return id ? document.getElementById(id)?.textContent : null;
};

describe("Combobox groups (web)", () => {
  it("names each group by its heading, skips an empty one, and walks one sequence across them", async () => {
    renderBox(<Harness groups={palette} options={undefined} />);
    const input = screen.getByRole("combobox");

    await userEvent.type(input, "a");
    expect(screen.getByRole("group", { name: "Channels" })).toBeTruthy();
    expect(screen.getByRole("group", { name: "In your library" })).toBeTruthy();
    expect(screen.queryByText("Nothing here")).toBeNull();

    await userEvent.keyboard("{ArrowDown}");
    expect(activeOption()).toBe("90s Action#42");
    // Down crosses the heading into the next group.
    await userEvent.keyboard("{ArrowDown}");
    expect(activeOption()).toBe("The Velvet CartographerIn library");
    await userEvent.keyboard("{ArrowDown}");
    expect(activeOption()).toBe("Pelican Point Motel");
    await userEvent.keyboard("{ArrowUp}{ArrowUp}");
    expect(activeOption()).toBe("90s Action#42");
    expect(document.activeElement).toBe(input);
  });

  it("picks from a later group with Enter, handing back the option", async () => {
    const onPick = vi.fn();
    renderBox(<Harness groups={palette} onPick={onPick} options={undefined} />);

    await userEvent.type(screen.getByRole("combobox"), "a{ArrowDown}{ArrowDown}{Enter}");
    expect(onPick).toHaveBeenCalledWith(expect.objectContaining({ value: "l1" }));
  });
});

describe("Combobox inline (web)", () => {
  const Palette = (props: ComponentProps<typeof Harness>) => (
    <Harness
      autoHighlight="always"
      endAdornment={<kbd>ESC</kbd>}
      groups={palette}
      inline
      listLabel="Search results"
      options={undefined}
      startAdornment={<span data-testid="lead" />}
      {...props}
    />
  );

  it("is open before anyone types, with the first option highlighted and no popup", () => {
    renderBox(<Palette />);
    const input = screen.getByRole("combobox", { name: "Location" });
    const listbox = screen.getByRole("listbox", { name: "Search results" });

    expect(input.getAttribute("aria-expanded")).toBe("true");
    expect(input.getAttribute("aria-controls")).toBe(listbox.id);
    expect(screen.getByTestId("lead")).toBeTruthy();
    expect(screen.getByText("ESC")).toBeTruthy();
    // In place, not portalled out of the render tree.
    expect(listbox.closest("[data-testid]")).toBeNull();
    expect(screen.getAllByRole("option")).toHaveLength(3);
    expect(activeOption()).toBe("90s Action#42");
  });

  it("steps through the groups with the arrows and wraps straight round at both ends", async () => {
    renderBox(<Palette />);
    screen.getByRole("combobox").focus();

    await userEvent.keyboard("{ArrowUp}");
    expect(activeOption()).toBe("Pelican Point Motel");
    await userEvent.keyboard("{ArrowDown}");
    expect(activeOption()).toBe("90s Action#42");
    await userEvent.keyboard("{ArrowDown}");
    expect(activeOption()).toBe("The Velvet CartographerIn library");
  });

  it("jumps to the first and last option with Home and End instead of moving the caret", async () => {
    renderBox(<Palette inputValue="velvet" />);
    const input = screen.getByRole<HTMLInputElement>("combobox");
    input.focus();
    input.setSelectionRange(3, 3);

    await userEvent.keyboard("{End}");
    expect(activeOption()).toBe("Pelican Point Motel");
    expect(input.selectionStart).toBe(3);
    await userEvent.keyboard("{ArrowUp}{Home}");
    expect(activeOption()).toBe("90s Action#42");
  });

  it("picks the highlighted option with Enter and a clicked option by click", async () => {
    const onPick = vi.fn();
    renderBox(<Palette onPick={onPick} />);
    screen.getByRole("combobox").focus();

    await userEvent.keyboard("{End}{Enter}");
    expect(onPick).toHaveBeenLastCalledWith(expect.objectContaining({ value: "l2" }));
    await userEvent.click(screen.getByRole("option", { name: /90s Action/ }));
    expect(onPick).toHaveBeenLastCalledWith(expect.objectContaining({ value: "c1" }));
    expect(screen.getByRole("listbox")).toBeTruthy();
  });

  it("moves the highlight to a hovered option, so the pointer and Enter agree", async () => {
    const onPick = vi.fn();
    renderBox(<Palette onPick={onPick} />);

    await userEvent.hover(screen.getByRole("option", { name: /Pelican/ }));
    expect(activeOption()).toBe("Pelican Point Motel");
    screen.getByRole("combobox").focus();
    await userEvent.keyboard("{Enter}");
    expect(onPick).toHaveBeenCalledWith(expect.objectContaining({ value: "l2" }));
  });

  it("hands the typed text up and puts the highlight back on the first option when the list changes", async () => {
    const onType = vi.fn();
    const { rerender } = renderBox(<Palette onType={onType} />);
    await userEvent.type(screen.getByRole("combobox"), "{End}b");
    expect(onType).toHaveBeenCalledWith("b");
    expect(activeOption()).toBe("90s Action#42");

    rerender(
      <LoomarrProvider>
        <Palette groups={[palette[2] as ComboboxGroup]} onType={onType} />
      </LoomarrProvider>,
    );
    expect(activeOption()).toBe("The Velvet CartographerIn library");
  });

  it("calls onEscape on every Escape, even with nothing to list, and keeps the text", async () => {
    const onEscape = vi.fn();
    const { rerender } = renderBox(<Palette inputValue="x" onEscape={onEscape} />);
    const input = screen.getByRole<HTMLInputElement>("combobox");
    input.focus();

    await userEvent.keyboard("{Escape}");
    expect(onEscape).toHaveBeenCalledTimes(1);
    expect(input.value).toBe("x");

    rerender(
      <LoomarrProvider>
        <Palette emptyText="No matches." groups={[]} inputValue="x" onEscape={onEscape} />
      </LoomarrProvider>,
    );
    await userEvent.keyboard("{Escape}");
    expect(onEscape).toHaveBeenCalledTimes(2);
  });

  it("leaves Escape alone when there is no onEscape, so a dialog around it still closes", async () => {
    const outer = vi.fn();
    const listen = (event: KeyboardEvent) => event.key === "Escape" && outer();
    window.addEventListener("keydown", listen);
    renderBox(<Palette />);
    screen.getByRole("combobox").focus();

    await userEvent.keyboard("{Escape}");
    window.removeEventListener("keydown", listen);
    expect(outer).toHaveBeenCalledTimes(1);
  });

  it("stops an Escape it handles from reaching the window", async () => {
    const outer = vi.fn();
    const listen = (event: KeyboardEvent) => event.key === "Escape" && outer();
    window.addEventListener("keydown", listen);
    renderBox(<Palette onEscape={() => undefined} />);
    screen.getByRole("combobox").focus();

    await userEvent.keyboard("{Escape}");
    window.removeEventListener("keydown", listen);
    expect(outer).not.toHaveBeenCalled();
  });

  it("says nothing matched without drawing an empty listbox, and the field does not point at one", () => {
    renderBox(<Palette emptyText="No matches for “zz”." groups={[]} inputValue="zz" />);
    const input = screen.getByRole("combobox");

    expect(screen.queryByRole("listbox")).toBeNull();
    expect(screen.getByRole("status").textContent).toBe("No matches for “zz”.");
    expect(input.getAttribute("aria-controls")).toBeNull();
    expect(input.getAttribute("aria-expanded")).toBe("false");
    expect(input.getAttribute("aria-activedescendant")).toBeNull();
  });

  it("does nothing on Home, End, the arrows or Enter with no options", async () => {
    const onPick = vi.fn();
    renderBox(<Palette emptyText="No matches." groups={[]} onPick={onPick} />);
    screen.getByRole("combobox").focus();

    await userEvent.keyboard("{Home}{End}{ArrowDown}{ArrowUp}{Enter}");
    expect(onPick).not.toHaveBeenCalled();
    expect(activeOption()).toBeNull();
  });

  it("takes its name from a visible label", () => {
    renderBox(
      <>
        <span id="find">Find anything</span>
        <Palette aria-labelledby="find" />
      </>,
    );
    expect(screen.getByRole("combobox", { name: "Find anything" })).toBeTruthy();
  });
});

describe("Combobox floating list keys (web)", () => {
  it("keeps Home and End on the text caret, where the list is a popup", async () => {
    renderBox(<Harness inputValue="seattle" />);
    const input = screen.getByRole<HTMLInputElement>("combobox");
    await userEvent.type(input, "{ArrowDown}");
    const before = activeOption();

    await userEvent.keyboard("{End}");
    expect(input.selectionStart).toBe(input.value.length);
    await userEvent.keyboard("{Home}");
    expect(input.selectionStart).toBe(0);
    expect(activeOption()).toBe(before);
  });

  it("closes the list on the first Escape and calls onEscape on the next", async () => {
    const onEscape = vi.fn();
    renderBox(<Harness onEscape={onEscape} />);
    const input = screen.getByRole("combobox");

    await userEvent.type(input, "x");
    expect(screen.getByRole("listbox")).toBeTruthy();
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(onEscape).not.toHaveBeenCalled();

    await userEvent.keyboard("{Escape}");
    expect(onEscape).toHaveBeenCalledTimes(1);
  });

  // Base UI hides the rest of the page from assistive tech while a typeable list is open (a screen
  // reader stays in the list; Tab closes it). That is why the Open story shows one field alone: axe
  // reads a second focusable field under that aria-hidden as a violation. It must come back on close.
  it("hides the rest of the page only while its list is open", async () => {
    renderBox(
      <>
        <Harness label="Language" />
        <Harness label="Location" />
      </>,
    );
    const other = screen.getByRole("combobox", { name: "Language" });
    const hidden = () => other.closest("[aria-hidden='true']") !== null;

    await userEvent.click(screen.getByRole("combobox", { name: "Location" }));
    await userEvent.keyboard("{ArrowDown}");
    expect(screen.getByRole("listbox")).toBeTruthy();
    expect(hidden()).toBe(true);

    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(hidden()).toBe(false);
  });
});
