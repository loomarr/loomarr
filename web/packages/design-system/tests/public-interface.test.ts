import { describe, expect, it } from "vitest";

import {
  ActivityIndicator,
  ArtworkFrame,
  Badge,
  BrandLaunch,
  BrandLockup,
  BrandMark,
  BrandWordmark,
  brandContract,
  FocusSurface,
  Icon,
  iconography,
  icons,
  LoomarrProvider,
  ProgressTrack,
  Screen,
  SignalLoader,
  Skeleton,
  Surface,
  semanticColors,
  Text,
  typography,
} from "../index";

describe("design-system public interface", () => {
  it("exposes semantic values and universal primitives without Tamagui leaking to callers", () => {
    expect([
      LoomarrProvider,
      ActivityIndicator,
      Screen,
      Surface,
      FocusSurface,
      Text,
      Badge,
      ArtworkFrame,
      ProgressTrack,
      SignalLoader,
      Skeleton,
      BrandMark,
      BrandWordmark,
      BrandLockup,
      BrandLaunch,
      brandContract,
      Icon,
      icons.play,
      iconography,
      semanticColors,
      typography,
    ]).not.toContain(undefined);
  });
});
