import type { Intent } from "@loomarr/api";
import { intentSchema } from "@loomarr/core";
import { describe, expect, it } from "vitest";

// The intent form's zod schema and the API's `Intent` must agree on FIELD NAMES, and
// nothing was checking that: the schema said `maxAcquire` where the wire says
// `maxAcquisitions`, and `runtimeTarget` where it says `runtimeTargetMin`. Both parse
// fine and both serialize to JSON the server silently ignores — a user's acquisition
// cap and runtime target just vanished. Types alone didn't catch it because the form
// never had to satisfy `Intent`.
//
// ⚠ THE COMPILE-TIME HALF MOVED TO THE SCHEMA ITSELF. `intentSchema` is now `.pick()`ed off the
// generated `submitProposalBody`, so a lookalike name cannot be WRITTEN — it fails as
// `Type 'true' is not assignable to type 'never'` at the pick, before this file is reached.
// That guard covers all three schemas in packages/core; this test only ever covered one.
//
// The assignability check below is kept anyway, and deliberately: `.pick()` guarantees the
// NAMES exist on the wire, while this guarantees the parsed OUTPUT still satisfies `Intent`
// after the hand-authored `.extend()` rewrites the value types. Those are different claims —
// an `.extend()` that changed `maxAcquisitions` to a string would sail past `.pick()`.
type IntentForm = ReturnType<typeof intentSchema.parse>;
const _formIsAValidIntent: (form: IntentForm) => Intent = (form) => form;

describe("intent schema ↔ API contract", () => {
  it("parses into the wire's field names, not lookalikes", () => {
    const parsed = intentSchema.parse({
      description: "90s action movies",
      era: "1990s",
      tone: "high-energy",
      runtimeTargetMin: 180,
      maxAcquisitions: 7,
    });

    // Spelled exactly as POST /v1/proposals accepts them.
    expect(parsed.runtimeTargetMin).toBe(180);
    expect(parsed.maxAcquisitions).toBe(7);
    // The old lookalikes must not survive anywhere in the parsed body.
    expect(Object.keys(parsed)).not.toContain("runtimeTarget");
    expect(Object.keys(parsed)).not.toContain("maxAcquire");
  });

  it("still requires a describable intent", () => {
    expect(intentSchema.safeParse({ description: "hi" }).success).toBe(false);
    expect(intentSchema.safeParse({ description: "90s action movies" }).success).toBe(true);
  });
});
