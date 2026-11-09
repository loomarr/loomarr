import { LoomarrProvider } from "@loomarr/design-system";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PhoneHeader } from "./phone-header";

describe("PhoneHeader", () => {
  it("puts one Search control in the header, and pressing it opens search", async () => {
    const onOpenSearch = vi.fn();
    render(
      <LoomarrProvider theme="dark">
        <PhoneHeader onOpenSearch={onOpenSearch} />
      </LoomarrProvider>,
    );
    const header = screen.getByRole("banner");
    await userEvent.click(within(header).getByRole("button", { name: "Search" }));
    expect(onOpenSearch).toHaveBeenCalledTimes(1);
  });
});
