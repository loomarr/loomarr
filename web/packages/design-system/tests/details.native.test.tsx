// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Details, LoomarrProvider } from "../index";

// The native Details (details.tsx): the summary is a button that shows or hides the content.

afterEach(cleanup);

const inTheme = (ui: React.ReactNode) => <LoomarrProvider>{ui}</LoomarrProvider>;

describe("Details (native)", () => {
  it("is closed unless told otherwise, and its summary opens and closes it", async () => {
    const onToggle = vi.fn();
    render(
      inTheme(
        <Details onToggle={onToggle} summary="Technical details">
          The body
        </Details>,
      ),
    );
    const summary = screen.getByRole("button", { name: "Technical details" });
    expect(screen.queryByText("The body")).toBeNull();

    await userEvent.click(summary);
    expect(screen.getByText("The body")).toBeTruthy();
    expect(summary.getAttribute("aria-expanded")).toBe("true");
    expect(onToggle).toHaveBeenLastCalledWith(true);

    await userEvent.click(summary);
    expect(screen.queryByText("The body")).toBeNull();
    expect(onToggle).toHaveBeenLastCalledWith(false);
  });

  it("follows a caller that changes `open`", () => {
    const { rerender } = render(
      inTheme(
        <Details open={false} summary="More">
          The body
        </Details>,
      ),
    );

    rerender(
      inTheme(
        <Details open summary="More">
          The body
        </Details>,
      ),
    );

    expect(screen.getByText("The body")).toBeTruthy();
  });
});
