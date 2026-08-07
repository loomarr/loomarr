import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TunePanel } from "./tune-panel";

// The auto-file policy panel (§10 V42, the v2 mock's `toggleAuto` block).
//
// ⚠ These assert the PATCH BODIES, not that a click rendered something. The chips write real
// settings that change what happens to clips without a human — a control that looks right and
// writes the wrong key is the failure worth catching, and no rendering assertion sees it.

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const setting = (key: string, value: string, provenance = "default") => ({
  key,
  value,
  provenance,
  kind: key.includes("confidence") ? "int" : "bool",
  group: "filler",
  doc: "",
  advanced: false,
  secret: false,
  set: true,
});

const stubFetch = (over: { settings?: unknown[] } = {}) => {
  const calls: { method: string; url: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const u = String(url);
      const method = init?.method ?? "GET";
      calls.push({ method, url: u, body: init?.body ? JSON.parse(init.body as string) : undefined });
      if (u.includes("/v1/settings")) {
        return Promise.resolve(
          jsonResponse(200, {
            settings: over.settings ?? [
              setting("filler.autofile.min_confidence", "85"),
              setting("filler.autofile.normalize_loudness", "false"),
            ],
          }),
        );
      }
      return Promise.resolve(jsonResponse(200, {}));
    }),
  );
  return calls;
};

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const renderPanel = (filed = 12, needsYou = 3) =>
  render(<TunePanel filed={filed} needsYou={needsYou} />, { wrapper: makeWrapper() });

afterEach(() => vi.unstubAllGlobals());

describe("TunePanel", () => {
  // ⚠ COLLAPSED by default, matching the mock. The policy is context, not the task — an
  // expanded settings form above the queue would push the clips that need a human below the fold.
  it("starts collapsed and opens on Tune", async () => {
    stubFetch();
    renderPanel();

    expect(screen.queryByText(/file automatically above/i)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /tune/i }));
    expect(await screen.findByText(/file automatically above/i)).toBeInTheDocument();
    // The button becomes its own inverse, as the mock's `tuneLabel` does.
    expect(screen.getByRole("button", { name: /hide settings/i })).toBeInTheDocument();
  });

  it("marks the stored confidence as the pressed chip", async () => {
    stubFetch({
      settings: [
        setting("filler.autofile.min_confidence", "95"),
        setting("filler.autofile.normalize_loudness", "false"),
      ],
    });
    renderPanel();
    await userEvent.click(screen.getByRole("button", { name: /tune/i }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "95%" })).toHaveAttribute("aria-pressed", "true");
    });
    expect(screen.getByRole("button", { name: "85%" })).toHaveAttribute("aria-pressed", "false");
  });

  // ⚠ THE regression this file exists for: the exact key and the exact value.
  it("writes the confidence chip to filler.autofile.min_confidence", async () => {
    const calls = stubFetch();
    renderPanel();
    await userEvent.click(screen.getByRole("button", { name: /tune/i }));
    await screen.findByRole("button", { name: "75%" });

    await userEvent.click(screen.getByRole("button", { name: "75%" }));

    await waitFor(() => {
      const patch = calls.find((c) => c.method === "PATCH");
      expect(patch?.body).toEqual({ edits: { "filler.autofile.min_confidence": "75" } });
    });
  });

  it("writes the loudness toggle as a string bool", async () => {
    const calls = stubFetch();
    renderPanel();
    await userEvent.click(screen.getByRole("button", { name: /tune/i }));
    const box = await screen.findByRole("checkbox", { name: /normalize loudness/i });

    await userEvent.click(box);

    await waitFor(() => {
      const patch = calls.find((c) => c.method === "PATCH");
      expect(patch?.body).toEqual({ edits: { "filler.autofile.normalize_loudness": "true" } });
    });
  });

  // ⚠ The label must name the SHIPPED target. The mock says −16; the system normalises to −23,
  // and a number on screen that the system does not use is a lie rather than a design delta.
  it("names the shipped −23 LUFS target, not the mock's −16", async () => {
    stubFetch();
    renderPanel();
    await userEvent.click(screen.getByRole("button", { name: /tune/i }));

    expect(await screen.findByText(/−23 LUFS/)).toBeInTheDocument();
    expect(screen.queryByText(/−16 LUFS/)).not.toBeInTheDocument();
  });

  // ⚠ This rewrites the operator's files irreversibly. A checkbox alone is the wrong affordance
  // for that, so the warning appears exactly when the setting is ON.
  it("warns that files are replaced only while the loudness toggle is on", async () => {
    stubFetch();
    const { unmount } = renderPanel();
    await userEvent.click(screen.getByRole("button", { name: /tune/i }));
    await screen.findByRole("checkbox", { name: /normalize loudness/i });
    expect(screen.queryByText(/can't be recovered/i)).not.toBeInTheDocument();
    unmount();

    stubFetch({
      settings: [
        setting("filler.autofile.min_confidence", "85"),
        setting("filler.autofile.normalize_loudness", "true"),
      ],
    });
    renderPanel();
    await userEvent.click(screen.getByRole("button", { name: /tune/i }));

    expect(await screen.findByText(/can't be recovered/i)).toBeInTheDocument();
  });

  // ⚠ An env-pinned key is read-only, as it is everywhere else (§3.1). A chip that looks
  // clickable and then silently loses to the environment is worse than a disabled one.
  it("disables the chips when the key is pinned by the environment", async () => {
    stubFetch({
      settings: [
        setting("filler.autofile.min_confidence", "85", "env"),
        setting("filler.autofile.normalize_loudness", "false"),
      ],
    });
    renderPanel();
    await userEvent.click(screen.getByRole("button", { name: /tune/i }));

    await waitFor(() => expect(screen.getByRole("button", { name: "75%" })).toBeDisabled());
  });

  it("summarises what the current policy does to the queue in front of it", async () => {
    stubFetch();
    renderPanel(12, 3);
    await userEvent.click(screen.getByRole("button", { name: /tune/i }));

    expect(await screen.findByText(/files 12 clips without asking/i)).toBeInTheDocument();
    expect(screen.getByText(/3 come to you/i)).toBeInTheDocument();
  });

  it("says nothing needs you when the queue is empty", async () => {
    stubFetch();
    renderPanel(4, 0);
    await userEvent.click(screen.getByRole("button", { name: /tune/i }));

    expect(await screen.findByText(/nothing needs you/i)).toBeInTheDocument();
  });
});
