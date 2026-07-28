import type { SetupCheck } from "@loomarr/api";
import { describe, expect, it } from "vitest";
import { deriveStepStatuses, firstIncompleteStep, isConnectionId, isStepDone, resolveStep } from "./steps";

const check = (name: string, ok: boolean): SetupCheck => ({ name, ok });
const green: SetupCheck[] = [
  check("media_server", true),
  check("tunarr", true),
  check("livetv", true),
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

  it("maps each wiring check to its own step (livetv has no step — auto-wired on save)", () => {
    const ctx = { checks: green, isAuthenticated: true };
    expect(isStepDone("library", ctx)).toBe(true);
    expect(isStepDone("library", { ...ctx, checks: [check("tunarr_library", false)] })).toBe(false);
    // livetv is not a step — its check is reflected on the Tunarr connection, not gated here.
  });

  it("marks current, skipped, done and pending distinctly", () => {
    const statuses = deriveStepStatuses({
      checks: [check("media_server", true), check("tunarr", true)],
      isAuthenticated: true,
      currentId: "library",
      skipped: new Set(["users"]),
    });
    expect(statuses.bootstrap).toBe("done");
    expect(statuses.checklist).toBe("done");
    expect(statuses.library).toBe("current");
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

// `?step=` / `?conn=` deep links (§13). The URL is a REQUEST; server truth still decides.
describe("wizard deep links", () => {
  const fresh = { checks: [], isAuthenticated: false }; // nothing done — frontier is bootstrap
  const wired = { checks: green, isAuthenticated: true }; // everything done — frontier is channel

  it("falls back to the resume point when no step is requested", () => {
    expect(resolveStep(undefined, fresh)).toBe("bootstrap");
    expect(resolveStep(undefined, wired)).toBe("channel");
  });

  // ⚠ THE STRANDING CASE, which is why this clamps rather than honouring the link. The
  // wizard offers only Back/Continue, the rail is not clickable, and Continue on an unmet
  // required step is disabled — so landing someone past the frontier is a screen with no
  // way forward at all.
  it("clamps a link that points past what the server says is done", () => {
    expect(resolveStep("users", fresh)).toBe("bootstrap");
    expect(resolveStep("channel", fresh)).toBe("bootstrap");
    expect(resolveStep("library", { checks: [], isAuthenticated: true })).toBe("checklist");
  });

  // The support case: "open your Connections step" on an install that is already further on.
  it("honours a link to an earlier step, which is the point of sharing one", () => {
    expect(resolveStep("bootstrap", wired)).toBe("bootstrap");
    expect(resolveStep("checklist", wired)).toBe("checklist");
  });

  it("honours a link to exactly the frontier", () => {
    expect(resolveStep("bootstrap", fresh)).toBe("bootstrap");
    expect(resolveStep("channel", wired)).toBe("channel");
  });

  // A link from an older release naming a step that no longer exists lands somewhere real
  // rather than on a blank screen — the same narrowing `?tab=` uses.
  it("treats an unknown step as no request at all", () => {
    expect(resolveStep("nope", wired)).toBe("channel");
    expect(resolveStep("", fresh)).toBe("bootstrap");
  });

  // `?conn=` is constrained to the Connections step's OWN sub-items, so a link cannot ask to
  // reveal a block that does not exist there.
  it("accepts only real connection ids", () => {
    expect(isConnectionId("tunarr")).toBe(true);
    expect(isConnectionId("media_server")).toBe(true);
    expect(isConnectionId("ai")).toBe(true);
    expect(isConnectionId("library")).toBe(false); // a STEP id, not a connection
    expect(isConnectionId("nope")).toBe(false);
    expect(isConnectionId(undefined)).toBe(false);
  });
});
