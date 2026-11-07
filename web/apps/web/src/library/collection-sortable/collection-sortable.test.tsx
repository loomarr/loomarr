import { LibraryCollectionEditor, type LibraryCollectionEditorProps } from "@loomarr/ui";
import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { SORTABLE } from "./collection-sortable";

const props = (onMove: LibraryCollectionEditorProps["onMove"]): LibraryCollectionEditorProps => ({
  saved: true,
  name: "Harbour nights",
  onNameChange: vi.fn(),
  description: "",
  onDescriptionChange: vi.fn(),
  members: [
    { id: "90000801", name: "Harbour Lights" },
    { id: "90000802", name: "Paper Kites" },
  ],
  onMove,
  onRemove: vi.fn(),
  onSearch: vi.fn(() => Promise.resolve([])),
  onAdd: vi.fn(),
  usedBy: [],
  onBack: vi.fn(),
  onCancel: vi.fn(),
  onSave: vi.fn(),
  onDelete: vi.fn(),
  busy: false,
  sortable: SORTABLE,
});

// The drag and drop the editor's rows ride on: each row's grip is a real button a keyboard can
// pick up, named for its title.
describe("collection sortable", () => {
  it("gives every row a grip named for its title that a keyboard can pick up", () => {
    render(<LibraryCollectionEditor {...props(vi.fn())} />);

    const grip = screen.getByRole("button", { name: "Move Paper Kites" });
    expect(grip).toHaveAttribute("aria-roledescription", "sortable");
    expect(screen.getByRole("button", { name: "Move Harbour Lights" })).toBeInTheDocument();
  });

  it("doesn't move a row that's picked up and dropped where it was", () => {
    const onMove = vi.fn();
    render(<LibraryCollectionEditor {...props(onMove)} />);

    const grip = screen.getByRole("button", { name: "Move Paper Kites" });
    fireEvent.keyDown(grip, { code: "Space", key: " " });
    fireEvent.keyDown(grip, { code: "Space", key: " " });
    expect(onMove).not.toHaveBeenCalled();
  });
});
