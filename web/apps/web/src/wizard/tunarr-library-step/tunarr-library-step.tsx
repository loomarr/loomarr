import { setupApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { ConnectStep } from "../connect-step";
import type { TunarrLibraryStepProps } from "./tunarr-library-step.type";

// Wizard step 5 — wire your library as *Tunarr's* media source (§13, §6). The opposite
// direction to step 3: this one points Tunarr at Emby/Jellyfin and triggers a scan.
// It closes a real silent-failure gap — without the scan, a channel's slots find no
// Tunarr program and quietly degrade to dead air while the rest of the wizard reads
// all-green, which is exactly why `tunarr_library` is its own check.
const TunarrLibraryStep = ({ check }: TunarrLibraryStepProps) => {
  const queryClient = useQueryClient();
  const connect = setupApi.useTunarrConnect({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: setupApi.getSetupStatusQueryKey() }),
    },
  });

  return (
    <ConnectStep
      check={check}
      cta="Wire Tunarr to your library"
      onConnect={() => connect.mutate()}
      isPending={connect.isPending}
      error={connect.error}
    >
      Points Tunarr at your Emby/Jellyfin library and scans it, so channels play real programs instead of dead
      air. Safe to run again. Loomarr only scans when you ask.
    </ConnectStep>
  );
};

export { TunarrLibraryStep };
