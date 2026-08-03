import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { NavTabs } from "./nav-tabs";

// A plain anchor stand-in — NavTabs takes the router's Link injected so it stays
// router-agnostic (see nav-tabs.tsx). Mirrors the story's `storyLink`.
const anchorLink = ({
  to,
  className,
  children,
  ...rest
}: {
  to: string;
  className?: string;
  children?: React.ReactNode;
  [key: string]: unknown;
}) => (
  <a href={to} className={className} {...rest}>
    {children}
  </a>
);

const tabs = [
  { id: "approval", label: "Needs approval", to: "/queue/approval", count: 3 },
  { id: "flight", label: "In flight", to: "/queue/flight", count: 12 },
  { id: "history", label: "History", to: "/queue/history", count: 14 },
];

describe("NavTabs", () => {
  it("renders a real link per tab with its count", () => {
    render(<NavTabs tabs={tabs} activeId="flight" linkComponent={anchorLink} label="Queue sections" />);

    const links = screen.getAllByRole("link");
    expect(links).toHaveLength(3);
    expect(screen.getByRole("link", { name: /Needs approval/ })).toHaveAttribute("href", "/queue/approval");
    expect(screen.getByRole("link", { name: /Needs approval/ })).toHaveTextContent("3");
    expect(screen.getByRole("link", { name: /History/ })).toHaveTextContent("14");
  });

  // ⚠ Omitted, not "0" — see the count field's own note: absence means counting does not
  // apply to this tab, and "0" means zero things are here. A tab with no count carries none.
  it("omits the count pill entirely when a tab has none", () => {
    render(
      <NavTabs
        tabs={[{ id: "sources", label: "Sources", to: "/filler/sources" }]}
        activeId="sources"
        linkComponent={anchorLink}
        label="Filler sections"
      />,
    );
    expect(screen.getByRole("link", { name: "Sources" })).toHaveTextContent(/^Sources$/);
  });

  // These are NAVIGATION, not the ARIA tablist pattern (nav-tabs.tsx's own warning): the
  // active destination is marked with aria-current, and only it points at a panel — an
  // inactive tab's target panel is not even mounted.
  it("marks the active destination with aria-current and aria-controls", () => {
    render(<NavTabs tabs={tabs} activeId="history" linkComponent={anchorLink} label="Queue sections" />);

    const active = screen.getByRole("link", { name: /History/ });
    expect(active).toHaveAttribute("aria-current", "page");
    expect(active).toHaveAttribute("aria-controls", "panel-history");

    const inactive = screen.getByRole("link", { name: /In flight/ });
    expect(inactive).not.toHaveAttribute("aria-current");
    expect(inactive).not.toHaveAttribute("aria-controls");
  });

  it("passes search params through to the link", () => {
    render(
      <NavTabs
        tabs={[{ id: "catalog", label: "Catalog", to: "/filler", search: { view: "list" } }]}
        activeId="catalog"
        linkComponent={({ to, search, children }) => (
          <a href={to} data-search={JSON.stringify(search)}>
            {children}
          </a>
        )}
        label="Filler sections"
      />,
    );
    expect(screen.getByRole("link", { name: "Catalog" })).toHaveAttribute(
      "data-search",
      JSON.stringify({ view: "list" }),
    );
  });
});
