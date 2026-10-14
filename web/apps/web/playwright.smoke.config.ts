import { defineConfig } from "@playwright/test";
import { DESKTOP, DETERMINISM } from "./playwright.shared";

// THE MAINTAINER SMOKE (§21 Definition of Done, second half).
//
// This is NOT part of comprehensive verification or `make e2e`, and must never be: those suites are
// hermetic and mock every external service (AGENTS.md — unit tests never touch the
// network). This one is the opposite by design. It drives the REAL embedded SPA served
// by a REAL loomarr binary against a REAL media server, TMDB, Ollama, and a purpose-built
// throwaway Tunarr — because the per-phase gates have twice been green while the seams
// between them were unwired (§21 phase 12.5 exists for exactly that reason).
//
// Run it deliberately: `make smoke`. It needs the smoke stack up (see the target), and it
// is expected to be run supervised, on a machine whose stack you are willing to touch.
//
// No screenshots: this proves BEHAVIOR against live services, and a snapshot of real
// library data would be nondeterministic by definition. The visual contract is the
// storybook suite's job.
const BASE = process.env.SMOKE_BASE_URL ?? "http://127.0.0.1:8090";

export default defineConfig({
  ...DETERMINISM,
  testDir: "./tests/smoke",
  // Strictly serial: the run tells ONE story — a new operator setting up a new install —
  // and each step depends on the last. Parallelism would race the shared server state.
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  // Real services are slower than mocks: an LLM suggestion run and a Tunarr scan are
  // both measured in tens of seconds, not milliseconds.
  timeout: 180_000,
  expect: { timeout: 30_000 },
  use: { ...DETERMINISM.use, baseURL: BASE },
  projects: [{ name: "desktop", use: DESKTOP }],
});
