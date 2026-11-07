import {
  getCleanupFillerStorageMockHandler,
  getFillerReadinessMockHandler,
  getMeMockHandler,
  getPreviewFillerStorageCleanupMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { readiness } from "@/test/fixtures/filler";
import { me } from "@/test/fixtures/users";
import { server } from "@/test/msw/server";
import { SourceStorageCard } from "./source-storage-card";

const storage = {
  automatic: true,
  state: "healthy",
  totalBytes: 500 * 1024 ** 3,
  freeBytes: 200 * 1024 ** 3,
  managedBytes: 1024 ** 3,
  reservedBytes: 0,
  filesystemReservedBytes: 0,
  softBudgetBytes: 20 * 1024 ** 3,
  hardReserveBytes: 10 * 1024 ** 3,
  availableBytes: 20 * 1024 ** 3,
} as const;

const renderCard = () =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <SourceStorageCard />
    </QueryClientProvider>,
  );

describe("SourceStorageCard", () => {
  it("offers to free unfinished downloads when the host drive's reserve paused new filler", async () => {
    let cleaned = false;
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      getFillerReadinessMockHandler(
        readiness({ storage: { ...storage, state: "paused", pausedBy: "host_reserve" } }),
      ),
      getPreviewFillerStorageCleanupMockHandler({ items: 2, bytes: 3 * 1024 ** 2 }),
      getCleanupFillerStorageMockHandler(() => {
        cleaned = true;
        return {
          removedItems: 2,
          removedBytes: 3 * 1024 ** 2,
          failedItems: 0,
          remaining: { items: 0, bytes: 0 },
        };
      }),
    );
    renderCard();

    expect(await screen.findByText(/New filler is paused so this drive keeps 10.0 GB free/)).toBeVisible();
    await userEvent.click(await screen.findByRole("button", { name: "Free 3.0 MB" }));

    expect(cleaned).toBe(true);
  });

  it("points a folder with unreadable capacity at the folder settings", async () => {
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      getFillerReadinessMockHandler(
        readiness({ storage: { ...storage, state: "paused", pausedBy: "capacity_unavailable" } }),
      ),
    );
    renderCard();

    expect(await screen.findByRole("link", { name: "Choose folder" })).toHaveAttribute(
      "href",
      "/filler/settings/folders",
    );
  });

  it("renders the readiness failure in the card's place", async () => {
    server.use(
      getMeMockHandler(me({ name: "Admin" })),
      http.get("*/v1/filler/readiness", () =>
        HttpResponse.json(
          { title: "Unavailable", status: 503, detail: "Readiness is down." },
          { status: 503 },
        ),
      ),
    );
    renderCard();

    expect(await screen.findByRole("button", { name: "Try again" })).toBeVisible();
    expect(screen.queryByRole("region", { name: "Storage" })).not.toBeInTheDocument();
  });
});
