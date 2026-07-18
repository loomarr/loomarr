import { setupApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { ConnectStep } from "../connect-step";
import type { LiveTvStepProps } from "./livetv-step.type";

// Wizard step 3 — wire Tunarr into the family's TV guide (§13, §6). This is the step
// that makes Loomarr's channels *appear on the TV*: it registers Tunarr as an M3U
// tuner + XMLTV guide source in Emby/Jellyfin. Idempotent, and never done unasked —
// Loomarr only performs the one-time setup an operator would otherwise do by hand.
const LiveTvStep = ({ check }: LiveTvStepProps) => {
  const queryClient = useQueryClient();
  const connect = setupApi.useLivetvConnect({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: setupApi.getSetupStatusQueryKey() }),
    },
  });

  return (
    <ConnectStep
      check={check}
      cta="Connect Tunarr to the guide"
      onConnect={() => connect.mutate()}
      isPending={connect.isPending}
      error={connect.error}
    >
      Adds Tunarr to Emby/Jellyfin as a tuner and guide source, so your channels show up in the TV guide
      alongside everything else. Safe to run more than once.
    </ConnectStep>
  );
};

export { LiveTvStep };
