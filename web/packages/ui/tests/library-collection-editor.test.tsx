// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { LibraryCollectionEditor, type LibraryCollectionEditorProps } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type El = {
  click(): void;
  getAttribute(n: string): string | null;
  textContent: string | null;
};
type Host = Parameters<typeof createRoot>[0] & {
  querySelector(s: string): El | null;
  querySelectorAll(s: string): ArrayLike<El>;
  remove(): void;
  textContent: string | null;
};
const doc = (
  globalThis as unknown as {
    document: { body: { append(n: unknown): void }; createElement(t: string): Host };
  }
).document;

const render = (overrides: Partial<LibraryCollectionEditorProps>) => {
  const props: LibraryCollectionEditorProps = {
    saved: true,
    name: "Harbour nights",
    onNameChange: vi.fn(),
    description: "",
    onDescriptionChange: vi.fn(),
    members: [
      { id: "90000801", name: "Harbour Lights", year: 1999 },
      { id: "90000802", name: "Paper Kites" },
    ],
    onMove: vi.fn(),
    onRemove: vi.fn(),
    onSearch: vi.fn(() => Promise.resolve([])),
    onAdd: vi.fn(),
    usedBy: [],
    onBack: vi.fn(),
    onCancel: vi.fn(),
    onSave: vi.fn(),
    onDelete: vi.fn(),
    busy: false,
    ...overrides,
  };
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  act(() =>
    root.render(
      <LoomarrProvider theme="dark">
        <LibraryCollectionEditor {...props} />
      </LoomarrProvider>,
    ),
  );
  const button = (name: string) => {
    const hit = Array.from(host.querySelectorAll('[role="button"], button')).find(
      (n) => n.textContent?.trim() === name || n.getAttribute("aria-label") === name,
    );
    if (!hit) throw new Error(`missing button: ${name}`);
    return hit;
  };
  const disabled = (name: string) => {
    const b = button(name);
    return b.getAttribute("aria-disabled") === "true" || b.getAttribute("disabled") !== null;
  };
  const cleanup = () => {
    act(() => root.unmount());
    host.remove();
  };
  return { host, props, button, disabled, cleanup };
};

describe("LibraryCollectionEditor", () => {
  it("lists the titles in order and removes one", () => {
    const { host, props, button, cleanup } = render({});
    expect(host.textContent).toContain("Harbour Lights");
    expect(host.textContent).toContain("1999");
    expect(host.textContent).toContain("Drag to change the order.");
    act(() => button("Remove Paper Kites").click());
    expect(props.onRemove).toHaveBeenCalledWith("90000802");
    cleanup();
  });

  it("says no channel uses it, and deletes it", () => {
    const { host, props, button, cleanup } = render({});
    expect(host.textContent).toContain("No channel uses this collection.");
    act(() => button("Delete collection").click());
    expect(props.onDelete).toHaveBeenCalled();
    cleanup();
  });

  it("won't delete while a channel uses it, and says which", () => {
    const { host, disabled, cleanup } = render({ usedBy: [{ name: "Saturday Morning", number: 4 }] });
    expect(host.textContent).toContain(
      "On Saturday Morning (channel 4) uses this. Take it off the channel first.",
    );
    expect(disabled("Delete collection")).toBe(true);
    cleanup();
  });

  it("is a new collection until it has a name, with nothing to delete", () => {
    const { host, disabled, cleanup } = render({ saved: false, name: "", members: [] });
    expect(host.textContent).toContain("New collection");
    expect(host.textContent).not.toContain("Delete collection");
    expect(disabled("Save")).toBe(true);
    cleanup();
  });

  it("says when a save didn't work", () => {
    const { host, props, button, cleanup } = render({ failed: "save" });
    expect(host.textContent).toContain("Couldn't save just now. Try again in a moment.");
    act(() => button("Save").click());
    expect(props.onSave).toHaveBeenCalled();
    cleanup();
  });

  it("goes back to Collections", () => {
    const { props, button, cleanup } = render({});
    act(() => button("Collections").click());
    expect(props.onBack).toHaveBeenCalled();
    act(() => button("Cancel").click());
    expect(props.onCancel).toHaveBeenCalled();
    cleanup();
  });

  it("hands the rows to the app's drag and drop", () => {
    const seen: string[] = [];
    const { host, cleanup } = render({
      sortable: {
        List: ({ ids, children }) => (
          <div data-ids={ids.join(",")} data-testid="list">
            {children}
          </div>
        ),
        Row: ({ id, children }) => {
          seen.push(id);
          return <div>{children}</div>;
        },
        Grip: ({ label, children }) => (
          <button aria-label={label} type="button">
            {children}
          </button>
        ),
      },
    });
    expect(host.querySelector('[data-testid="list"]')?.getAttribute("data-ids")).toBe("90000801,90000802");
    expect(seen).toEqual(["90000801", "90000802"]);
    expect(host.querySelector('[aria-label="Move Harbour Lights"]')).not.toBeNull();
    cleanup();
  });
});
