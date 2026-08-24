import { describe, expect, it } from "vitest";

import { ClientPlatformProof, ClientShell, PairingShell, ProgrammeCard, StatePanel } from "../index";

describe("ui public interface", () => {
  it("exports the shared scaffold surface", () => {
    expect(ClientPlatformProof).toBeTypeOf("function");
    expect(ClientShell).toBeTypeOf("function");
    expect(PairingShell).toBeTypeOf("function");
    expect(ProgrammeCard).toBeTypeOf("function");
    expect(StatePanel).toBeTypeOf("function");
  });
});
