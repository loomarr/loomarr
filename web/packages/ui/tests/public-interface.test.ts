import { describe, expect, it } from "vitest";

import { ClientPlatformProof, ClientShell, PairingShell, ProgrammeCard } from "../index";

describe("ui public interface", () => {
  it("exports the shared scaffold surface", () => {
    expect(ClientPlatformProof).toBeTypeOf("function");
    expect(ClientShell).toBeTypeOf("function");
    expect(PairingShell).toBeTypeOf("function");
    expect(ProgrammeCard).toBeTypeOf("function");
  });
});
