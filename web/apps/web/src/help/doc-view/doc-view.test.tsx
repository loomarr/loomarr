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

describe("DocView images", () => {
  it("shows a page's diagram as an <img> from the binary, not the repo path", () => {
    render(<DocView markdown="![How it flows](../diagrams/generated/architecture.svg)" />);
    const img = screen.getByRole("img", { name: "How it flows" });
    expect(img.tagName).toBe("IMG");
    expect(img).toHaveAttribute("src", "/v1/docs/diagrams/architecture.svg");
  });

  it("shows a page's screenshot as an <img> from the binary, not the repo path", () => {
    render(<DocView markdown="![The guide](../images/screenshots/guide-dark.webp)" />);
    expect(screen.getByRole("img", { name: "The guide" })).toHaveAttribute(
      "src",
      "/v1/docs/screenshots/guide-dark.webp",
    );
  });

  it("never fetches any other image: Help works air-gapped", () => {
    render(<DocView markdown="![A remote picture](https://example.com/x.png)" />);
    expect(screen.queryByRole("img")).toBeNull();
    expect(screen.getByText("A remote picture")).toBeInTheDocument();
  });

  it("never renders inline SVG from the Markdown", () => {
    const { container } = render(<DocView markdown={"<svg><script>alert(1)</script></svg>\n\ntext"} />);
    expect(container.querySelector("svg, script")).toBeNull();
  });
});
