import { useRef } from "react";
import { useAuth } from "@/auth/use-auth";
import { FillerSources } from "@/components/loomarr/filler/filler-sources";
import { LocalSourceForm } from "./local-source-form";
import { ProviderSourceFinder } from "./provider-source-finder";
import { SourceDetailSheet } from "./source-detail-sheet";
import { SourceStorageCard } from "./source-storage-card";
import type { SourcesPanelProps } from "./sources-panel.type";
import { useSourceActions } from "./use-source-actions";

// SourcesPanel — the Sources tab of the filler page (§10 V35/V37/V38c): registered sources
// (folders, libraries, archive.org, YouTube), each switchable, fetchable and (if `removable`)
// forgettable, plus per-source search for archive.org and the "Add a source" form.
//
// ⚠ `useSyncFiller` and `useTagFiller` were called here and are gone (V38c). Both were
// whole-catalog buttons in the header; the sync runs on its own schedule and drains the watch
// folder every pass, and tagging follows it. The per-source "Look for new clips" in the source
// drawer uses its own route — so nothing here is left unreachable.
const SourcesPanel = ({
  sources,
  sourcesError,
  selectedSourceID,
  onSelectSource,
  onCloseSource,
}: SourcesPanelProps) => {
  const { isAdmin } = useAuth();
  const actions = useSourceActions({ onSourceRemoved: onCloseSource });
  const sourceTrigger = useRef<HTMLElement>(null);
  const selectedSource = sources.find((source) => source.id === selectedSourceID);

  return (
    <div className="flex flex-col gap-6">
      <SourceStorageCard />

      <FillerSources
        sources={sources}
        onSelect={(source) => {
          sourceTrigger.current =
            document.activeElement instanceof HTMLElement ? document.activeElement : null;
          onSelectSource(source.id);
        }}
        selectedId={selectedSourceID}
        onToggleEnabled={(id, enabled) => actions.updateSource(id, { enabled })}
        toggling={actions.updatingSourceID ?? null}
        onToggleProvider={actions.toggleProvider}
        togglingProvider={actions.togglingProvider ?? null}
        renderLocalSetup={isAdmin ? <LocalSourceForm /> : undefined}
        error={actions.error ?? sourcesError ?? null}
        renderProviderSetup={(provider) => {
          if (provider.kind !== "archive" && provider.kind !== "youtube") return undefined;
          return <ProviderSourceFinder kind={provider.kind} enabled={provider.enabled} />;
        }}
      />

      <SourceDetailSheet
        source={selectedSource}
        actions={actions}
        finalFocus={sourceTrigger}
        onClose={onCloseSource}
      />
    </div>
  );
};

export { SourcesPanel };
