import type { TitleDTO } from "@loomarr/api/models/titleDTO";
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { render } from "@/test/render";
import { addedLine, NewThisWeek, newSince } from "./new-this-week";
import type { NewThisWeekProps } from "./new-this-week.type";

describe("New this week — who made the channel (#1663)", () => {
  const createdAtMs = Date.UTC(2026, 8, 29, 12); // a Tuesday
  const channel = { id: "c1", name: "Test", number: 22, createdAtMs } as const;

  it("names the requester, or says it was yours", () => {
    expect(addedLine({ ...channel, requestedBy: "Ada" } as never, "Bo", "UTC")).toBe(
      "Requested by Ada · added Tuesday",
    );
    expect(addedLine({ ...channel, requestedBy: "Bo" } as never, "Bo", "UTC")).toBe(
      "Your request · added Tuesday",
    );
  });

  it("says only when a hand-made channel arrived", () => {
    expect(addedLine(channel as never, "Bo", "UTC")).toBe("Added Tuesday");
  });

  // The titles query key must hold still between renders within the hour.
  it("quantises the week's start to the hour", () => {
    const now = Date.UTC(2026, 8, 28, 12, 34, 56);
    expect(newSince(now)).toBe(newSince(now + 60_000));
    expect(newSince(now)).toBe(Date.UTC(2026, 8, 21, 12));
  });
});

const title = (key: string, over: Record<string, unknown> = {}): TitleDTO =>
  ({
    key,
    mediaType: "movie",
    name: key,
    state: "available",
    ...over,
  }) as TitleDTO;

const renderNewThisWeek = async (props: Partial<NewThisWeekProps>) => {
  const rootRoute = createRootRoute({
    component: () => <NewThisWeek titles={[]} channels={[]} since={0} {...props} />,
  });
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  render(<RouterProvider router={router} />);
  await screen.findByRole("heading", { name: "New this week" });
};

describe("New this week — rendering (#1822 evidence)", () => {
  it("renders a new channel row separately from the posters, not as a grid column", async () => {
    await renderNewThisWeek({
      channels: [{ id: "c1", name: "Field Notes", number: 19, createdAtMs: Date.now() } as never],
      titles: [title("t1")],
    });
    const row = screen.getByRole("link", { name: /Field Notes/ });
    const section = screen.getByRole("heading", { name: "New this week" }).closest("section");
    // The row is its own direct child of the section — a sibling of the poster grid, never a
    // column inside it (the geometry table: "New channel: Separate row").
    expect(row.parentElement).toBe(section);
    expect(row.className).not.toContain("grid");
  });

  // #2763: the header said "3 new channels" and listed one.
  it("lists every new channel the header counts", async () => {
    const now = Date.now();
    await renderNewThisWeek({
      channels: [
        { id: "c1", name: "Field Notes", number: 19, createdAtMs: now - 3000 } as never,
        { id: "c2", name: "Late Horror", number: 7, createdAtMs: now - 1000 } as never,
        { id: "c3", name: "Cartoon Club", number: 3, createdAtMs: now - 2000 } as never,
      ],
    });
    expect(screen.getByText("3 new channels")).toBeInTheDocument();
    const rows = screen.getAllByRole("link", { name: /New channel/ });
    expect(rows.map((row) => row.textContent)).toEqual([
      expect.stringContaining("Late Horror"),
      expect.stringContaining("Cartoon Club"),
      expect.stringContaining("Field Notes"),
    ]);
  });

  it("shows few recent titles without a lead channel", async () => {
    await renderNewThisWeek({ titles: [title("t1"), title("t2")] });
    expect(screen.getByText("t1")).toBeInTheDocument();
    expect(screen.getByText("t2")).toBeInTheDocument();
    expect(screen.queryByText("New channel")).not.toBeInTheDocument();
  });

  it("caps many recent titles at five posters", async () => {
    await renderNewThisWeek({ titles: Array.from({ length: 14 }, (_, i) => title(`t${i}`)) });
    expect(screen.getAllByText(/^t\d+$/)).toHaveLength(5);
  });

  it("gives a title with no artwork the line-motif fallback, with its channel's monogram when known", async () => {
    await renderNewThisWeek({
      titles: [title("t1", { channels: [{ id: "c1", name: "Field Notes", number: 19 }] })],
    });
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(screen.getByText("FN")).toBeInTheDocument();
  });

  it("wraps a very long title instead of truncating it", async () => {
    const longTitle =
      "The remarkably long story of a lighthouse keeper and the extraordinary journey home and on";
    await renderNewThisWeek({ titles: [title("t1", { name: longTitle })] });
    const node = screen.getByText(longTitle);
    expect(node.className).toContain("break-words");
  });
});
