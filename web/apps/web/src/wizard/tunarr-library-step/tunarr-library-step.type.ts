import type { SetupCheck } from "@loomarr/api/models/setupCheck";

interface TunarrLibraryStepProps {
  // The `tunarr_library` check from setup/status — its own check precisely because a
  // missing scan otherwise fails silently into dead air (§6).
  check?: SetupCheck;
}

export type { TunarrLibraryStepProps };
