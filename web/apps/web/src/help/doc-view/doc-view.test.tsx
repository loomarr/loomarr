import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { DocView } from "./doc-view";

describe("DocView links", () => {
  it("routes an internal cross-page link through onNavigate (§13)", async () => {
    const onNavigate = vi.fn();
    render(
      <DocView markdown="See the [approval](concepts#approval-the-one-gate) gate." onNavigate={onNavigate} />,
    );

    // Internal links render as buttons, not raw anchors — a raw href would resolve
    // relative to the current path and break.
    await userEvent.click(screen.getByRole("button", { name: /approval/i }));
    expect(onNavigate).toHaveBeenCalledWith("concepts", "approval-the-one-gate");
  });

  it("opens an external link in a new tab, not through the router", () => {
    render(<DocView markdown="Get a [TMDB key](https://www.themoviedb.org)." onNavigate={vi.fn()} />);
    const link = screen.getByRole("link", { name: /tmdb key/i });
    expect(link).toHaveAttribute("href", "https://www.themoviedb.org");
    expect(link).toHaveAttribute("target", "_blank");
  });

  it("renders headings with stable ids so deep-links land (§13 anchor contract)", () => {
    render(<DocView markdown={"## Media server\n\ntext"} />);
    // The id is the slug the API's docHref points at.
    expect(document.getElementById("media-server")).not.toBeNull();
  });
});
