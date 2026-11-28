import { describe, expect, it } from "vitest";
import { PLAYOUT_INTERNAL, PLAYOUT_TUNARR, wizardSteps } from "@/wizard/steps";
import { COPY, continueGated, SKIPPABLE } from "./wizard-rules";

const everyStep = [...wizardSteps(PLAYOUT_INTERNAL), ...wizardSteps(PLAYOUT_TUNARR)].map((s) => s.id);

describe("wizard rules", () => {
  it("has copy for every step of both backends", () => {
    for (const id of everyStep) expect(COPY[id]?.title, id).toBeTruthy();
  });

  it("lets only the optional steps be skipped", () => {
    expect([...new Set(everyStep.filter((id) => SKIPPABLE.has(id)))].sort()).toEqual([
      "library",
      "media",
      "users",
    ]);
  });

  it("gates Continue on required steps and on the media step, not on other skippable ones", () => {
    expect(continueGated("checklist")).toBe(true);
    expect(continueGated("media")).toBe(true);
    expect(continueGated("library")).toBe(false);
    expect(continueGated("users")).toBe(false);
  });
});
