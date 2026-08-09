import type { ClipDTO, FillerIncomingOutputBody, IncomingAskDTO } from "@loomarr/api";
import {
  getFillerIncomingMockHandler,
  getSettingsListMockHandler,
  getTagFillerClipMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { IncomingTab } from "./incoming-tab";

// IncomingTab owns the review queue and its four filing mutations. It was extracted from
// `filler-page.tsx`, where all of it mounted on every tab — so a member reading the Catalog
// still paid for a queue that is admin-only server-side.
//
// ⚠ These tests assert the REQUEST BODIES, not just that a click did something. The BE's
// `UpdateClipTags` writes era and audience unconditionally, so a confirm that sends a bare
// `{era}` silently wipes audience. That is a data-loss bug no rendering assertion can see, and
// this call site reproduced it once already. `category` is NOT part of the body (§10 V45a): it
// is a derived shadow of the taxonomy tags, and this confirm never touches tags at all.

// ⚠ Typed as IncomingAskDTO, which surfaced two MISSING required fields (`kind`, `reason`) the
// untyped stub happily accepted.
const ASK: IncomingAskDTO = {
  kind: "commercial",
  reason: "era-guess",
  path: "a3/f9/abc.mp4",
  hash: "hash-abc",
  name: "Toy ad",
  suggestedEra: 1993,
  audience: "kids",
  category: "toys",
  confidence: 80,
  durationMs: 30_000,
};

// The tagged clip the PATCH answers with. Only its shape matters here — the assertions are on
// what was SENT — but it has to be a real ClipDTO, which is nine required fields.
const TAGGED: ClipDTO = {
  hash: "hash-abc",
  name: "Toy ad",
  kind: "commercial",
  durationMs: 30_000,
  era: 1993,
  audience: "kids",
  tagged: true,
  aiTagged: false,
  playCount: 0,
  playsCounted: true,
};

// ⚠ The stub this replaced ended in `return Promise.resolve(jsonResponse(200, {}))` — a catch-all
// answering any other url with an empty object — and its assertions then searched
// `calls.find((c) => c.method === "PATCH")`. Both are the weakness this migration removes: the
// catch-all could not fail, and a method filter would match a PATCH to ANY endpoint. Since the
// whole point of this file is that the PATCH carries the right BODY to the right route, binding
// the handler to `*/v1/filler/tags` is the assertion the old test could not make.
const stubIncoming = (incoming: Partial<FillerIncomingOutputBody> = {}) => {
  const body: FillerIncomingOutputBody = {
    asks: [ASK],
    reels: [],
    recentlyFiled: [],
    // ⚠ `pipeline` and `rejected` are REQUIRED and the old stub supplied NEITHER — two more
    // fields an untyped catch-all let through.
    pipeline: [],
    rejected: [],
    total: 1,
    ...incoming,
  };
  const patches: unknown[] = [];
  server.use(
    getFillerIncomingMockHandler(body),
    // ⚠ The tab also reads /v1/settings, which the OLD catch-all answered with `{}` — so this
    // code path ran against an empty settings payload and nothing said so. The guard named it.
    getSettingsListMockHandler({ settings: [], features: {} }),
    getTagFillerClipMockHandler(async ({ request }) => {
      patches.push(await request.json());
      return TAGGED;
    }),
  );
  return { patches };
};

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const renderTab = (onEditTags = vi.fn()) => {
  render(<IncomingTab onEditTags={onEditTags} />, { wrapper: makeWrapper() });
  return { onEditTags };
};

describe("IncomingTab", () => {
  it("fetches the incoming queue itself rather than being handed it", async () => {
    stubIncoming();
    renderTab();
    // The whole point of the extraction: the tab is self-sufficient, so the shell no longer
    // unpacks a queue for a tab that may not be showing. Rendering the ask IS the proof the
    // fetch happened — and an unmatched request would now fail the test by name, where the old
    // catch-all would have answered it silently.
    expect(await screen.findByText("Toy ad")).toBeInTheDocument();
  });

  // ⚠ THE regression this file exists for. A bare `{era}` blanks audience on the server, so the
  // confirm must carry the clip's current audience alongside the era. `category` must NOT be in
  // the body — it's a derived shadow (§10 V45a), and `PatchClipInputBody` has no such field any
  // more; sending it would be a stale field the server ignores at best.
  it("confirming an era sends audience too, never era alone, and never category", async () => {
    const { patches } = stubIncoming();
    renderTab();
    await screen.findByText("Toy ad");

    // "Looks right" is the CONFIRM affordance for a guessed era (the panel's own copy).
    await userEvent.click(await screen.findByRole("button", { name: /looks right/i }));

    await waitFor(() => {
      // §10 V45a: the clip is identified by `hash` in the body (no {id} URL segment) — this
      // PATCH goes through the single-clip tag route, which is hash-keyed. `ask.path` stays
      // the local UI key for `onEditTags`/`busyClip` (see the test below and the mixed-model
      // note in incoming-tab.tsx itself).
      expect(patches).toEqual([{ hash: "hash-abc", era: 1993, audience: "kids" }]);
    });
  });

  it("hands tag editing up to the shell, which owns the one dialog", async () => {
    stubIncoming();
    const { onEditTags } = renderTab();
    await screen.findByText("Toy ad");

    // For a clip with a GUESSED era the edit affordance reads "Not right" — the panel offers
    // "Add tags" only when there is no guess to reject.
    await userEvent.click(await screen.findByRole("button", { name: /not right/i }));

    expect(onEditTags).toHaveBeenCalledWith(ASK.path);
  });

  it("renders an empty queue without erroring", async () => {
    stubIncoming({ asks: [], total: 0 });
    renderTab();
    await waitFor(() => expect(screen.queryByText("Toy ad")).not.toBeInTheDocument());
  });
});
