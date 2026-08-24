import { describe, expect, it } from "vitest";

import { ClientPlatformProof, ProgrammeCard } from "../index";

describe("ui public interface", () => {
  it("exports the shared scaffold surface", () => {
    expect(ClientPlatformProof).toBeTypeOf("function");
    expect(ProgrammeCard).toBeTypeOf("function");
  });
});
