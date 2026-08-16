import type { SetupCheck } from "@loomarr/api";
import { describe, expect, it } from "vitest";
import {
  deriveStepStatuses,
  firstIncompleteStep,
  isConnectionId,
  isStepDone,
  PLAYOUT_INTERNAL,
  PLAYOUT_TUNARR,
  requiredChecks,
  resolveStep,
  wizardSteps,
} from "./steps";

const check = (name: string, ok: boolean): SetupCheck => ({ name, ok });
const green: SetupCheck[] = [
  check("media_server", true),
  check("tunarr", true),
  check("livetv", true),
  check("tunarr_library", true),
];
// Most of the pre-§9.1 assertions were written when Tunarr was mandatory, so they pass a
// backend explicitly now. `tunarr` keeps the old shape honest; `internal` is the new default.
const onTunarr = { isAuthenticated: true, backend: PLAYOUT_TUNARR } as const;

describe("wizard step derivation", () => {
  it("treats bootstrap as done once an admin session exists", () => {
    expect(isStepDone("bootstrap", { checks: [], isAuthenticated: false })).toBe(false);
    expect(isStepDone("bootstrap", { checks: [], isAuthenticated: true })).toBe(true);
  });

  it("completes Connections on the required checks only", () => {
    // Optional integrations failing must not block the wizard (config-design §6).
    const partial = [check("media_server", true), check("tunarr", true), check("llm", false)];
    expect(isStepDone("checklist", { ...onTunarr, checks: partial })).toBe(true);
    expect(isStepDone("checklist", { ...onTunarr, checks: [check("media_server", true)] })).toBe(false);
  });

  it("maps each wiring check to its own step (livetv has no step, it auto-wires on save)", () => {
    const ctx = { ...onTunarr, checks: green };
    expect(isStepDone("library", ctx)).toBe(true);
    expect(isStepDone("library", { ...ctx, checks: [check("tunarr_library", false)] })).toBe(false);
    // livetv is not a step — its check is reflected on the Tunarr connection, not gated here.
  });

  it("marks current, skipped, done and pending distinctly", () => {
    const statuses = deriveStepStatuses({
      ...onTunarr,
      checks: [check("media_server", true), check("tunarr", true)],
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
    expect(firstIncompleteStep({ checks: [], isAuthenticated: true })).toBe("playout");
    expect(firstIncompleteStep({ checks: [], isAuthenticated: true, publicURL: "http://loomarr:8080" })).toBe(
      "checklist",
    );
    expect(
      firstIncompleteStep({ checks: green, isAuthenticated: true, publicURL: "http://loomarr:8080" }),
    ).toBe("channel");
  });

  it("never blocks on an optional step when resuming", () => {
    // users is optional — a fully wired install resumes past it to the first channel.
    expect(firstIncompleteStep({ ...onTunarr, checks: green })).not.toBe("users");
  });
});

// The playout choice reshapes the wizard (design §13, §9.1).
//
// ⚠ THE DEFECT THIS EXISTS FOR: `tunarr` was a hardcoded blocking check with its own wiring
// step, so an operator on the DEFAULT internal path was asked to install and connect a second
// server they would never use, and could not continue until they did.
describe("the playout choice reshapes the wizard", () => {
  const ids = (backend: "internal" | "tunarr") => wizardSteps(backend).map((s) => s.id);

  it("requires Loomarr's reachable address only when internal playout owns the stream", () => {
    const base = { checks: [], isAuthenticated: true };
    expect(isStepDone("playout", { ...base, backend: PLAYOUT_INTERNAL, publicURL: "" })).toBe(false);
    expect(
      isStepDone("playout", {
        ...base,
        backend: PLAYOUT_INTERNAL,
        publicURL: "http://loomarr:8080",
      }),
    ).toBe(true);
    expect(isStepDone("playout", { ...base, backend: PLAYOUT_TUNARR, publicURL: "" })).toBe(true);
  });

  it("drops Tunarr's wiring step on the internal path and keeps it on the Tunarr path", () => {
    expect(ids(PLAYOUT_INTERNAL)).not.toContain("library");
    expect(ids(PLAYOUT_TUNARR)).toContain("library");
  });

  it("asks about playout on both paths, before Connections", () => {
    for (const backend of [PLAYOUT_INTERNAL, PLAYOUT_TUNARR] as const) {
      const order = ids(backend);
      expect(order).toContain("playout");
      expect(order.indexOf("playout")).toBeLessThan(order.indexOf("checklist"));
      // And after Admin: the choice is admin-gated like everything past bootstrap.
      expect(order.indexOf("bootstrap")).toBeLessThan(order.indexOf("playout"));
    }
  });

  it("never lists Tunarr as a Connections sub-item", () => {
    const connsFor = (backend: "internal" | "tunarr") =>
      wizardSteps(backend)
        .find((s) => s.id === "checklist")
        ?.subItems?.map((i) => i.id) ?? [];
    // Neither path lists Tunarr here: its form lives on the Playout step now.
    expect(connsFor(PLAYOUT_INTERNAL)).not.toContain("tunarr");
    expect(connsFor(PLAYOUT_TUNARR)).not.toContain("tunarr");
    // The media server is on both: it is where the library lives either way.
    expect(connsFor(PLAYOUT_INTERNAL)).toContain("media_server");
  });

  // ⚠ The blocking half, which is what actually stranded people. An internal install can
  // never turn `tunarr` green, so requiring it was a gate with no key.
  it("requires only the media server when Loomarr does the streaming", () => {
    expect(requiredChecks(PLAYOUT_INTERNAL)).toEqual(["media_server"]);
    expect(requiredChecks(PLAYOUT_TUNARR)).toEqual(["media_server", "tunarr"]);
  });

  it("completes Connections without Tunarr on the internal path", () => {
    const onlyMediaServer = [check("media_server", true), check("tunarr", false)];
    expect(
      isStepDone("checklist", { checks: onlyMediaServer, isAuthenticated: true, backend: PLAYOUT_INTERNAL }),
    ).toBe(true);
    // Same checks, Tunarr path: still blocked, because there Tunarr really is required.
    expect(isStepDone("checklist", { ...onTunarr, checks: onlyMediaServer })).toBe(false);
  });

  // A removed step gets NO status rather than a "pending" the rail would draw as outstanding
  // work. "Not needed" is still an advertisement for work that is not part of this install.
  it("gives a removed step no status at all", () => {
    const statuses = deriveStepStatuses({
      checks: [check("media_server", true)],
      isAuthenticated: true,
      backend: PLAYOUT_INTERNAL,
      publicURL: "http://loomarr:8080",
      currentId: "checklist",
    });
    expect(statuses.library).toBeUndefined();
    expect(statuses.playout).toBe("done");
  });

  // An internal install resumes past Connections to Users/channel, never to a Tunarr step.
  it("resumes past Connections on the internal path once the media server is green", () => {
    const ctx = {
      checks: [check("media_server", true)],
      isAuthenticated: true,
      backend: PLAYOUT_INTERNAL,
      publicURL: "http://loomarr:8080",
    };
    expect(firstIncompleteStep(ctx)).toBe("channel");
  });

  // Defaults to internal when the settings query has not resolved yet, matching the registry
  // default — so a first paint never briefly demands Tunarr and then takes it back.
  it("treats an unresolved backend as internal", () => {
    expect(requiredChecks(PLAYOUT_INTERNAL)).toEqual(["media_server"]);
    expect(isStepDone("checklist", { checks: [check("media_server", true)], isAuthenticated: true })).toBe(
      true,
    );
  });

  // A link to a step this backend does not have is as unreal as one to a step that never
  // existed, and falls through to the frontier the same way.
  it("does not honour a link to a step the chosen backend removed", () => {
    const ctx = { checks: green, isAuthenticated: true, backend: PLAYOUT_INTERNAL };
    expect(resolveStep("library", ctx)).not.toBe("library");
  });

  // Tunarr is no longer a Connections sub-item on EITHER path: its form moved to the Playout
  // step, under the choice that makes it relevant. So `?conn=tunarr` opens nothing anywhere.
  it("no longer treats tunarr as a connection sub-item", () => {
    expect(isConnectionId("tunarr", PLAYOUT_TUNARR)).toBe(false);
    expect(isConnectionId("tunarr", PLAYOUT_INTERNAL)).toBe(false);
    expect(isConnectionId("media_server", PLAYOUT_INTERNAL)).toBe(true);
  });
});

// `?step=` / `?conn=` deep links (§13). The URL is a REQUEST; server truth still decides.
describe("wizard deep links", () => {
  const fresh = { checks: [], isAuthenticated: false }; // nothing done — frontier is bootstrap
  const wired = { checks: green, isAuthenticated: true, publicURL: "http://loomarr:8080" }; // everything done — frontier is channel

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
    expect(
      resolveStep("library", { checks: [], isAuthenticated: true, publicURL: "http://loomarr:8080" }),
    ).toBe("checklist");
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
    // tunarr is deliberately NOT one: its form moved to the Playout step.
    expect(isConnectionId("tunarr", PLAYOUT_TUNARR)).toBe(false);
    expect(isConnectionId("media_server")).toBe(true);
    expect(isConnectionId("ai")).toBe(true);
    expect(isConnectionId("library")).toBe(false); // a STEP id, not a connection
    expect(isConnectionId("nope")).toBe(false);
    expect(isConnectionId(undefined)).toBe(false);
  });
});
