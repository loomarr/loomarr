import { describe, expect, it } from "vitest";

import {
  ChannelIdentity,
  ClientNavigation,
  ClientPlatformProof,
  ClientShell,
  clientBackDestination,
  GuideExperience,
  GuideSurface,
  ModalOverlay,
  PairingShell,
  ProgrammeCard,
  ProgrammeIdentity,
  StatePanel,
  TransientOverlay,
} from "../index";

describe("ui public interface", () => {
  it("exports the shared scaffold surface", () => {
    expect(ClientPlatformProof).toBeTypeOf("function");
    expect(ClientShell).toBeTypeOf("function");
    expect(PairingShell).toBeTypeOf("function");
    expect(ProgrammeCard).toBeTypeOf("function");
    expect(ChannelIdentity).toBeTypeOf("function");
    expect(ClientNavigation).toBeTypeOf("function");
    expect(GuideSurface).toBeTypeOf("function");
    expect(GuideExperience).toBeTypeOf("function");
    expect(clientBackDestination).toBeTypeOf("function");
    expect(ProgrammeIdentity).toBeTypeOf("function");
    expect(ModalOverlay).toBeTypeOf("function");
    expect(TransientOverlay).toBeTypeOf("function");
    expect(StatePanel).toBeTypeOf("function");
  });
});
