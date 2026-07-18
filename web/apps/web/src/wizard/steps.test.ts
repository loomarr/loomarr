import type { SetupCheck } from "@loomarr/api";
import { describe, expect, it } from "vitest";
import { deriveStepStatuses, firstIncompleteStep, isStepDone } from "./steps";

const check = (name: string, ok: boolean): SetupCheck => ({ name, ok });
const green: SetupCheck[] = [
  check("media_server", true),
  check("tunarr", true),
  check("livetv", true),
  check("webhook", true),
  check("tunarr_library", true),
];

describe("wizard step derivation", () => {
  it("treats bootstrap as done once an admin session exists", () => {
    expect(isStepDone("bootstrap", { checks: [], isAuthenticated: false })).toBe(false);
    expect(isStepDone("bootstrap", { checks: [], isAuthenticated: true })).toBe(true);
  });

  it("completes Connections on the required checks only", () => {
    const ctx = { isAuthenticated: true };
    // Optional integrations failing must not block the wizard (config-design §6).
    const partial = [check("media_server", true), check("tunarr", true), check("llm", false)];
    expect(isStepDone("checklist", { ...ctx, checks: partial })).toBe(true);
    expect(isStepDone("checklist", { ...ctx, checks: [check("media_server", true)] })).toBe(false);
  });

  it("maps each wiring check to its own step", () => {
    const ctx = { checks: green, isAuthenticated: true };
    expect(isStepDone("guide", ctx)).toBe(true);
    expect(isStepDone("webhooks", ctx)).toBe(true);
    expect(isStepDone("library", ctx)).toBe(true);
    expect(isStepDone("guide", { ...ctx, checks: [check("livetv", false)] })).toBe(false);
  });

  it("marks current, skipped, done and pending distinctly", () => {
    const statuses = deriveStepStatuses({
      checks: [check("media_server", true), check("tunarr", true)],
      isAuthenticated: true,
      currentId: "guide",
      skipped: new Set(["users"]),
    });
    expect(statuses.bootstrap).toBe("done");
    expect(statuses.checklist).toBe("done");
    expect(statuses.guide).toBe("current");
    expect(statuses.users).toBe("skipped");
    expect(statuses.channel).toBe("pending");
  });

  it("resumes at the first incomplete required step", () => {
    expect(firstIncompleteStep({ checks: [], isAuthenticated: false })).toBe("bootstrap");
    expect(firstIncompleteStep({ checks: [], isAuthenticated: true })).toBe("checklist");
    expect(firstIncompleteStep({ checks: green, isAuthenticated: true })).toBe("channel");
  });

  it("never blocks on an optional step when resuming", () => {
    // users is optional — a fully wired install resumes past it to the first channel.
    expect(firstIncompleteStep({ checks: green, isAuthenticated: true })).not.toBe("users");
  });
});
