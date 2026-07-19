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
// This is the compile-time half: if a schema field is renamed, `tsc` fails here (and
// `make fe` typechecks), rather than the value quietly going nowhere at runtime.
// Derived from the schema itself rather than importing zod, which apps/web does not
// depend on directly (the schemas live in packages/core).
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

    // Spelled exactly as POST /v1/suggestions accepts them.
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
