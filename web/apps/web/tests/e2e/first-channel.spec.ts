import AxeBuilder from "@axe-core/playwright";
import type { Intent, Proposal, ProposalDTO, ProposalFailure, ProposalJobDTO } from "@loomarr/api";
import { expect, test } from "@playwright/test";
import { installFirstChannelBackend } from "./first-channel-backend";

const INTENT = {
  description: "Saturday-morning cartoons like I watched as a kid — bright, silly, kid-safe",
  era: "1990s",
  tone: "playful",
} satisfies Intent;

const PROPOSAL = {
  intent: INTENT,
  channelName: "Saturday Morning Cartoons",
  rationale: "Bright, grounded animation from the requested era.",
  lineup: [{ name: "Animaniacs", year: 1993, mediaType: "series", tmdbId: 2429, inLibrary: true }],
  acquisitions: [{ name: "Gargoyles", year: 1994, mediaType: "series", tmdbId: 1719, inLibrary: false }],
  alternates: [],
  scores: { themeFit: 0.96, availabilityRatio: 0.5, eraBalance: 1, overall: 0.86 },
} satisfies Proposal;

const proposal = (jobId: string, status: ProposalDTO["status"] = "submitted"): ProposalDTO => ({
  id: `proposal-${jobId}`,
  jobId,
  status,
  proposal: PROPOSAL,
});

const job = (
  jobId: string,
  status: ProposalJobDTO["status"],
  options: { failure?: ProposalFailure; proposal?: ProposalDTO } = {},
): ProposalJobDTO => ({
  jobId,
  status,
  intent: INTENT,
  attempts: 1,
  createdAt: "2026-08-15T12:00:00Z",
  updatedAt: "2026-08-15T12:01:00Z",
  ...options,
});

test.describe("first channel acceptance", () => {
  test("preset survives durable generation and Watch starts transport only after the channel goes live", async ({
    page,
  }) => {
    const backend = await installFirstChannelBackend(page, {
      fillerEligible: 0,
      submittedJobs: [
        [
          job("job-first", "queued"),
          job("job-first", "running"),
          job("job-first", "done", { proposal: proposal("job-first") }),
        ],
      ],
    });

    await page.goto("/guide?preset=saturday-cartoons");
    await expect(page.getByLabel("Channel intent")).toHaveValue(INTENT.description);
    await page.getByRole("button", { name: /suggest a lineup/i }).click();

    await expect(page).toHaveURL(/\/guide\?jobId=job-first/);
    const running = page.locator('[role="status"][aria-busy="true"]');
    await expect(running).toHaveAttribute("aria-busy", "true");

    await expect(page.getByText("Animaniacs", { exact: true })).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText(/No break-ready filler yet.*play programs back-to-back/i)).toBeVisible();
    await expect(page.getByRole("button", { name: /approve & acquire/i })).toBeEnabled();
    expect(backend.state.jobReads["job-first"]).toBeGreaterThanOrEqual(3);
    expect(backend.state.eventConnections).toBeGreaterThan(0);

    await page.getByRole("button", { name: /approve & acquire/i }).click();
    await expect(page).toHaveURL(/\/channels\/ch-new\/watch$/);
    await expect(page.getByRole("heading", { name: "Watch Saturday Morning Cartoons" })).toBeAttached();
    await expect.poll(() => backend.state.approvals).toEqual(["proposal-job-first"]);

    // Opening the just-created building channel paints Watch, but never mints or fetches HLS
    // while there is no broadcast to join. This bounded quiet window is deliberate: a plain
    // immediate zero assertion can pass just before the player effect mints its URL.
    await page.waitForTimeout(750);
    expect.soft(backend.state.playURLRequests.building).toBe(0);
    expect.soft(backend.state.hlsRequests.building).toEqual([]);

    // The backend's authoritative channel read now says the asynchronous materialization is
    // complete. Reload models a browser with a silent SSE connection reconnecting from durable
    // state: Watch may start exactly one transport only after this live response.
    backend.goLive();
    await page.reload();
    await expect(page.getByRole("heading", { name: "Watch Saturday Morning Cartoons" })).toBeAttached();
    await expect(page.locator("video")).toBeVisible();
    await expect.poll(() => backend.state.playURLRequests.live).toBe(1);
    await expect.poll(() => backend.state.hlsRequests.live.length).toBeGreaterThan(0);
  });

  test("reload restores an active job and polling lands its review with SSE absent", async ({ page }) => {
    const backend = await installFirstChannelBackend(page, {
      initialJobs: {
        "job-reload": [
          job("job-reload", "queued"),
          job("job-reload", "running"),
          job("job-reload", "done", { proposal: proposal("job-reload") }),
        ],
      },
    });

    await page.goto("/guide?jobId=job-reload");
    await expect(page.getByRole("status")).toHaveAttribute("aria-busy", "true");
    await page.reload();

    await expect(page.getByText("Animaniacs", { exact: true })).toBeVisible({ timeout: 5_000 });
    await expect(page).toHaveURL(/\/guide\?jobId=job-reload/);
    expect(backend.state.eventConnections).toBeGreaterThan(0);
    expect(backend.state.jobReads["job-reload"]).toBeGreaterThanOrEqual(3);
  });

  test("a safe mobile failure focuses recovery and Edit restores the complete intent", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await installFirstChannelBackend(page, {
      initialJobs: {
        "job-edit": [
          job("job-edit", "failed", {
            failure: { code: "no_grounded_titles", message: "No grounded titles matched this request." },
          }),
        ],
      },
    });

    await page.goto("/guide?jobId=job-edit");
    const alert = page.getByRole("alert");
    await expect(alert).toBeVisible();
    await expect(alert).toBeFocused();
    await expect(alert).toContainText("No grounded titles matched this request.");
    await expect(page.getByText(/sk-secret|provider stack|diagnostic/i)).toHaveCount(0);

    const accessibility = await new AxeBuilder({ page }).include('[role="alert"]').analyze();
    expect(accessibility.violations).toEqual([]);

    await page.getByRole("button", { name: "Edit description" }).click();
    await expect(page).toHaveURL(/\/guide$/);
    await expect(page.getByLabel("Channel intent")).toHaveValue(INTENT.description);
    await page.getByRole("button", { name: "Add constraints" }).click();
    await expect(page.getByLabel("Era")).toHaveValue("1990s");
    await expect(page.getByLabel("Tone")).toHaveValue("playful");
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  });

  test("a timed-out request retries as a fresh durable job with the full intent", async ({ page }) => {
    const backend = await installFirstChannelBackend(page, {
      initialJobs: {
        "job-old": [
          job("job-old", "failed", {
            failure: { code: "timed_out", message: "Channel generation took too long." },
          }),
        ],
      },
      submittedJobs: [
        [job("job-retry", "queued"), job("job-retry", "done", { proposal: proposal("job-retry") })],
      ],
    });

    await page.goto("/guide?jobId=job-old");
    await expect(page.getByRole("link", { name: "Open AI settings" })).toBeVisible();
    await page.getByRole("button", { name: "Try again" }).click();

    await expect(page).toHaveURL(/\/guide\?jobId=job-retry/);
    await expect(page.getByText("Animaniacs", { exact: true })).toBeVisible({ timeout: 4_000 });
    expect(backend.state.submissions).toEqual([INTENT]);
  });

  test("a member sees the submitted review waiting for admin without decision controls", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await installFirstChannelBackend(page, {
      role: "member",
      initialJobs: {
        "job-member": [job("job-member", "done", { proposal: proposal("job-member") })],
      },
    });

    await page.goto("/guide?jobId=job-member");
    await expect(page.getByText("Waiting for admin approval.")).toBeVisible();
    await expect(page.getByRole("button", { name: /approve/i })).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^deny$/i })).toHaveCount(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  });
});
