import type { SetupCheck } from "@loomarr/api";

interface LiveTvStepProps {
  // The `livetv` check from setup/status — the BE probe is what reports success, not
  // the click (§6 "never silent").
  check?: SetupCheck;
}

export type { LiveTvStepProps };
