import { describe, expect, it } from "vitest";

import { ClipSelect, NeedsDetailsChip } from "../filler";
import { GuideExperience, GuideJourney, GuideSurface } from "../guide";
import {
  ChannelIdentity,
  ClientNavigation,
  ClientPlatformProof,
  ClientShell,
  clientBackDestination,
  ModalOverlay,
  ProgrammeCard,
  ProgrammeIdentity,
  StatePanel,
  TransientOverlay,
} from "../index";
import { DeviceDisconnectAction, PairingShell, pairingBackAvailable } from "../pairing";
import { SurfJourney, SurfRail, WatchingSurface } from "../watching";

describe("ui public interface", () => {
  it("exports the shared scaffold surface", () => {
    expect(ClientPlatformProof).toBeTypeOf("function");
    expect(ClientShell).toBeTypeOf("function");
    expect(ClipSelect).toBeTypeOf("function");
    expect(NeedsDetailsChip).toBeTypeOf("function");
    expect(DeviceDisconnectAction).toBeTypeOf("function");
    expect(PairingShell).toBeTypeOf("function");
    expect(pairingBackAvailable).toBeTypeOf("function");
    expect(ProgrammeCard).toBeTypeOf("function");
    expect(ChannelIdentity).toBeTypeOf("function");
    expect(ClientNavigation).toBeTypeOf("function");
    expect(GuideSurface).toBeTypeOf("function");
    expect(GuideExperience).toBeTypeOf("function");
    expect(GuideJourney).toBeTypeOf("function");
    expect(clientBackDestination).toBeTypeOf("function");
    expect(ProgrammeIdentity).toBeTypeOf("function");
    expect(ModalOverlay).toBeTypeOf("function");
    expect(TransientOverlay).toBeTypeOf("function");
    expect(StatePanel).toBeTypeOf("function");
    expect(SurfJourney).toBeTypeOf("function");
    expect(SurfRail).toBeTypeOf("function");
    expect(WatchingSurface).toBeTypeOf("function");
  });
});
