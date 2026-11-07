import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SourceItemPreviewDialog } from "./source-item-preview-dialog";

// The player URL is the provider's to build and arrives on the item, so the dialog plays what it is
// given and falls back to the original when it is given nothing.
describe("SourceItemPreviewDialog", () => {
  it("plays the embed the provider supplied and names the provider", () => {
    render(
      <SourceItemPreviewDialog
        item={{
          title: "Station break",
          url: "https://example.test/item",
          embedUrl: "https://example.test/embed/1",
        }}
        providerLabel="Registry Name"
        onClose={() => {}}
      />,
    );
    expect(screen.getByText(/This plays from Registry Name\./)).toBeVisible();
    expect(screen.getByTitle("Preview Station break")).toHaveAttribute("src", "https://example.test/embed/1");
  });

  it("offers only the original when the provider supplied no embed", () => {
    render(
      <SourceItemPreviewDialog
        item={{ title: "Station break", url: "https://example.test/item" }}
        providerLabel="Registry Name"
        onClose={() => {}}
      />,
    );
    expect(screen.queryByTitle("Preview Station break")).not.toBeInTheDocument();
    expect(screen.getByText(/cannot show the item here/)).toBeVisible();
    expect(screen.getByRole("link", { name: /Open original/ })).toHaveAttribute(
      "href",
      "https://example.test/item",
    );
  });
});
