import { setupApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { ConnectStep } from "@/wizard";

// The two one-click wiring actions, available for the LIFE of the install rather than
// only during first-run (config-design §5, design §13).
//
// They used to exist solely as wizard steps, which made them unreachable in two ways at
// once: the wizard's rail is not clickable and offers only Back/Continue, so a step that
// could not go green stranded the operator on that screen — and the *library* wiring sat
// behind the Live TV step, so an operator whose guide wiring failed could never reach the
// very thing that stops channels scheduling slots with no program (§6). Settings is
// already "the troubleshooting console for the life of the install" (§13); these belong
// here for exactly that reason.
//
// Both reuse the wizard's ConnectStep, so the affordance is identical in both places —
// idempotent, and the CHECK (not the click) reports success, because the BE's own probe
// is the only honest source of "this actually worked" (§6, never silent).
const WiringSettings = () => {
  const queryClient = useQueryClient();
  const status = setupApi.useSetupStatus();
  const checks = status.data?.status === 200 ? (status.data.data.checks ?? []) : [];
  const checkNamed = (name: string) => checks.find((c) => c.name === name);

  // Both actions change what the checklist reports, so both refresh it on success —
  // the same invalidation the wizard steps do.
  const invalidate = () => queryClient.invalidateQueries({ queryKey: setupApi.getSetupStatusQueryKey() });
  const livetv = setupApi.useLivetvConnect({ mutation: { onSuccess: invalidate } });
  const library = setupApi.useTunarrConnect({ mutation: { onSuccess: invalidate } });

  return (
    <div className="flex flex-col gap-6">
      <section className="flex flex-col gap-3">
        <h3 className="font-medium text-sm">Put your channels in the TV guide</h3>
        <ConnectStep
          check={checkNamed("livetv")}
          cta="Connect Tunarr to the guide"
          onConnect={() => livetv.mutate()}
          isPending={livetv.isPending}
          error={livetv.error}
        >
          Adds Tunarr to Emby/Jellyfin as a tuner and guide source, so your channels show up in the TV guide
          alongside everything else. Safe to run more than once.
        </ConnectStep>
      </section>

      <section className="flex flex-col gap-3">
        <h3 className="font-medium text-sm">Give Tunarr your library</h3>
        <ConnectStep
          check={checkNamed("tunarr_library")}
          cta="Wire Tunarr to your library"
          onConnect={() => library.mutate()}
          isPending={library.isPending}
          error={library.error}
        >
          Points Tunarr at your Emby/Jellyfin library, enables the movie and show folders, and triggers a scan
          — so your channels play real programs instead of dead air. Safe to run more than once; Loomarr never
          re-scans unasked.
        </ConnectStep>
      </section>
    </div>
  );
};

export { WiringSettings };
