import type { FillerReadinessDTO } from "@loomarr/api";
import { getFillerReadinessMockHandler, getMeMockHandler } from "@loomarr/api/msw";
import { StatusLabel } from "@loomarr/design-system";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { readiness } from "@/test/fixtures/filler";
import { me } from "@/test/fixtures/users";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { RouterHarness } from "@/test/story-utils";
import { FillerOverview, readinessAction } from "./filler-overview";

const show = (coverage: FillerReadinessDTO = readiness(), role: "admin" | "member" = "admin") => {
  server.use(getFillerReadinessMockHandler(coverage), getMeMockHandler(me({ role })));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <RouterHarness
      content={
        <QueryClientProvider client={client}>
          <FillerOverview />
        </QueryClientProvider>
      }
      initialPath="/filler"
    />,
  );
};

describe("FillerOverview", () => {
  it.each([
    ["enable_fetch", "downloads"],
    ["free_catalog_capacity", "storage"],
    ["choose_another_folder", "folders"],
    ["change_storage_limit", "storage"],
  ] as const)("opens the exact settings task for %s", (nextAction, section) => {
    expect(readinessAction(readiness({ ready: false, nextAction }))).toMatchObject({ section });
  });
  it.each([
    ["enable_fetch", "Turn on automatic sourcing", "Review automation", "/filler/settings/$section"],
    [
      "free_catalog_capacity",
      "Make room in the filler catalog",
      "Review limits",
      "/filler/settings/$section",
    ],
    ["free_disposable_space", "This drive needs room", "Review storage", "/filler/sources"],
    [
      "choose_another_folder",
      "Loomarr cannot check this drive",
      "Choose folder",
      "/filler/settings/$section",
    ],
    ["change_storage_limit", "Filler reached its allowance", "Review storage", "/filler/settings/$section"],
    ["retry_acquisition", "A download needs another try", "Open Incoming", "/filler/incoming"],
    ["retry_failed_work", "Some filler can be retried", "Open Incoming", "/filler/incoming"],
    ["review_incoming", "A few clips need your help", "Review clips", "/filler/incoming"],
    ["add_filler", "Add filler to get started", "Open sources", "/filler/sources"],
    [
      "improve_channel_coverage",
      "A channel needs better filler coverage",
      "Browse library",
      "/filler/library",
    ],
  ] as const)("presents the server-ranked %s action", (nextAction, title, label, to) => {
    expect(readinessAction(readiness({ ready: false, nextAction, actionCount: 2 }))).toMatchObject({
      title,
      label,
      to,
    });
  });

  it("renders no action when the readiness projection ranks none", () => {
    expect(readinessAction(readiness())).toBeUndefined();
  });

  it("renders the server-owned healthy answer quietly, without inventing an action", async () => {
    show();
    expect(await screen.findByText("Filler is working on its own")).toBeInTheDocument();
    // Normal is quiet (#1908): no badge at all, amber or otherwise.
    expect(screen.queryByText("Working automatically")).not.toBeInTheDocument();
    expect(screen.queryByText("Action recommended")).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /diagnostics|review clips/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/admission|admitted|readiness projection/i)).not.toBeInTheDocument();
  });

  it.each(["retry_acquisition", "retry_failed_work"] as const)(
    "sends %s to Incoming, where the held work is listed",
    async (nextAction) => {
      show(readiness({ ready: false, nextAction, actionCount: 2 }));
      expect(await screen.findByRole("link", { name: "Open Incoming" })).toHaveAttribute(
        "href",
        "/filler/incoming",
      );
    },
  );

  it("shows a member the state and who can act on it, with no admin-only button", async () => {
    show(readiness({ ready: false, nextAction: "review_incoming", actionCount: 3 }), "member");
    expect(await screen.findByText("A few clips need your help")).toBeInTheDocument();
    expect(screen.getByText("Action recommended")).toBeInTheDocument();
    expect(screen.getByText("An admin can sort this out from Filler › Manage.")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Review clips" })).not.toBeInTheDocument();
  });

  it("gives an admin the action and no member line", async () => {
    show(readiness({ ready: false, nextAction: "review_incoming", actionCount: 3 }));
    expect(await screen.findByRole("link", { name: "Review clips" })).toBeInTheDocument();
    expect(screen.queryByText(/An admin can sort this out/)).not.toBeInTheDocument();
  });

  it("marks a coverage card's channel link so the channel filler page can offer a way back", async () => {
    show(
      readiness({
        pool: {
          clips: 25,
          breakBody: 20,
          eligible: 18,
          untagged: 0,
          channels: [
            {
              channelId: "ch-42",
              name: "Saturday Mornings",
              number: 42,
              level: "exact",
              total: 12,
              durationMs: 360_000,
              categories: 3,
              brands: 7,
            },
          ],
        },
      }),
    );
    expect(await screen.findByRole("link", { name: "42 · Saturday Mornings" })).toHaveAttribute(
      "href",
      "/channels/ch-42/filler?from=filler",
    );
  });

  it("uses workspace readiness rather than a healthy admission subset", async () => {
    show(readiness({ ready: false, nextAction: "add_filler" }));

    expect(await screen.findByText("Add filler to get started")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open sources" })).toHaveAttribute("href", "/filler/sources");
    expect(screen.queryByText("Filler is working on its own")).not.toBeInTheDocument();
    expect(screen.queryByText("Working automatically")).not.toBeInTheDocument();
  });

  it("uses the current needs-help action without resurfacing legacy decision counts", async () => {
    show(readiness({ ready: false, nextAction: "review_incoming", actionCount: 4 }));

    expect(await screen.findByText("A few clips need your help")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Review clips" })).toHaveAttribute("href", "/filler/incoming");
    expect(screen.queryByRole("heading", { name: "Admission summary" })).not.toBeInTheDocument();
    expect(screen.queryByText("Needs judgment")).not.toBeInTheDocument();
    expect(screen.queryByText(/classified safely without a person/i)).not.toBeInTheDocument();
  });

  it("routes operational recovery to Incoming's work list, not the review action", async () => {
    show(readiness({ ready: false, nextAction: "retry_failed_work", actionCount: 2 }));
    expect(await screen.findByText("Some filler can be retried")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Review clips" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open Incoming" })).toHaveAttribute("href", "/filler/incoming");
  });

  it("keeps channel coverage separate from admission health", async () => {
    show(
      readiness({
        pool: {
          clips: 25,
          breakBody: 20,
          eligible: 18,
          untagged: 0,
          channels: [
            {
              channelId: "ch-42",
              name: "Saturday Mornings",
              number: 42,
              level: "widened",
              total: 12,
              durationMs: 360_000,
              categories: 3,
              brands: 7,
            },
          ],
        },
      }),
    );
    expect(await screen.findByText("6m playable · 12 clips")).toBeInTheDocument();
    expect(screen.getByText("3 categories · 7 brands")).toBeInTheDocument();
  });

  // The mock painted the worst rung green and the best amber. Green has to mean "good" and red
  // "nothing to play", so the tone is pinned per rung, with the mock's words. Asserted by the
  // rendered colour of the status dot (StatusLabel's public output, #2784), compared with a
  // reference dot of the intended tone rather than a hard-coded hex.
  it.each([
    ["exact", "Good match", "ok"],
    ["widened", "Years loosened", "warn"],
    ["audience", "Audience only", "warn"],
    ["bumper_card", "Bumpers only", "error"],
  ] as const)("badges a %s channel %s in the %s tone", async (level, label, tone) => {
    show(
      readiness({
        pool: {
          clips: 25,
          breakBody: 20,
          eligible: 18,
          untagged: 0,
          channels: [
            {
              channelId: "ch-42",
              name: "Saturday Mornings",
              number: 42,
              level,
              total: level === "bumper_card" ? 0 : 12,
              durationMs: level === "bumper_card" ? 0 : 360_000,
              categories: 0,
              brands: 0,
            },
          ],
        },
      }),
    );
    const word = await screen.findByText(label);
    const dot = word.previousElementSibling as HTMLElement;
    const reference = render(<StatusLabel tone={tone}>reference</StatusLabel>).getByText("reference")
      .previousElementSibling as HTMLElement;
    expect(dot.style.backgroundColor).not.toBe("");
    expect(dot.style.backgroundColor).toBe(reference.style.backgroundColor);
  });

  // #2733: channels drawing the same pool showed identical cards side by side. One card says it.
  it("shows one shared-pool card when every channel's coverage is the same", async () => {
    const channel = {
      level: "exact" as const,
      total: 12,
      durationMs: 360_000,
      categories: 3,
      brands: 7,
    };
    show(
      readiness({
        pool: {
          clips: 25,
          breakBody: 20,
          eligible: 18,
          untagged: 0,
          channels: [
            { ...channel, channelId: "ch-1", name: "Saturday Mornings", number: 1 },
            { ...channel, channelId: "ch-2", name: "Late Horror", number: 2 },
            { ...channel, channelId: "ch-3", name: "Cartoon Club", number: 3 },
          ],
        },
      }),
    );
    expect(await screen.findByText("All 3 channels")).toBeInTheDocument();
    expect(screen.getAllByText("6m playable · 12 clips")).toHaveLength(1);
    expect(screen.queryByRole("link", { name: "1 · Saturday Mornings" })).not.toBeInTheDocument();
  });

  it("keeps a card per channel when their coverage differs", async () => {
    const channel = { level: "exact" as const, durationMs: 360_000, categories: 3, brands: 7 };
    show(
      readiness({
        pool: {
          clips: 25,
          breakBody: 20,
          eligible: 18,
          untagged: 0,
          channels: [
            { ...channel, total: 12, channelId: "ch-1", name: "Saturday Mornings", number: 1 },
            { ...channel, total: 9, channelId: "ch-2", name: "Late Horror", number: 2 },
          ],
        },
      }),
    );
    expect(await screen.findByRole("link", { name: "1 · Saturday Mornings" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "2 · Late Horror" })).toBeInTheDocument();
    expect(screen.queryByText(/^All \d+ channels$/)).not.toBeInTheDocument();
  });

  it("says a bumper-only channel has only the card, not zero minutes", async () => {
    show(
      readiness({
        pool: {
          clips: 25,
          breakBody: 20,
          eligible: 18,
          untagged: 0,
          channels: [
            {
              channelId: "ch-7",
              name: "Late Horror",
              number: 7,
              level: "bumper_card",
              total: 0,
              durationMs: 0,
              categories: 0,
              brands: 0,
            },
          ],
        },
      }),
    );
    expect(await screen.findByText("Only a bumper card")).toBeInTheDocument();
    expect(screen.queryByText(/playable/)).not.toBeInTheDocument();
  });
});
