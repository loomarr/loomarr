import { fillerApi, settingsApi } from "@loomarr/api";
import { pluralize } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { RefreshCw, Sparkles } from "lucide-react";
import { useState } from "react";
import { useAuth } from "@/auth";
import { ClipCard, EmptyState, ErrorState } from "@/components/loomarr";
import { Button, Card, Input, Label, Select } from "@/components/ui";
import { ClipTagDialog } from "../clip-tag-dialog";
import { IngestPanel } from "../ingest-panel";

// FillerPage — the §10 clip catalog: browse, search, tag, sync, and (on the filler image)
// download. Filtering is client-driven but server-executed: the store already indexes
// these columns, so a query per filter change is cheaper and always correct, versus
// holding thousands of clips in memory to filter locally (§7.2 — no client index).
const FillerPage = () => {
  const queryClient = useQueryClient();
  const { isAdmin } = useAuth();
  const [q, setQ] = useState("");
  const [kind, setKind] = useState("");
  const [audience, setAudience] = useState("");
  const [untagged, setUntagged] = useState(false);
  const [tagging, setTagging] = useState<string>();

  const settings = settingsApi.useSettingsList();
  const features = settings.data?.status === 200 ? settings.data.data.features : undefined;
  const fillerConfigured = Boolean(features?.filler);

  const clips = fillerApi.useListFiller({
    ...(q ? { q } : {}),
    ...(kind ? { kind: kind as never } : {}),
    ...(audience ? { audience: audience as never } : {}),
    ...(untagged ? { untagged: true } : {}),
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerQueryKey() });

  const sync = fillerApi.useSyncFiller({ mutation: { onSuccess: invalidate } });
  const aiTag = fillerApi.useTagFiller({ mutation: { onSuccess: invalidate } });

  if (!fillerConfigured) {
    return (
      <div className="flex flex-col gap-6">
        <PageHeading />
        <EmptyState
          title="No filler folder configured"
          description="Point Loomarr at a folder of commercials and bumpers under Settings → Filler. Tunarr scans it directly — no media-server library involved."
        />
      </div>
    );
  }

  const rows = clips.data?.status === 200 ? (clips.data.data.clips ?? []) : undefined;
  const filtered = Boolean(q || kind || audience || untagged);

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <PageHeading />
        {isAdmin && (
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={sync.isPending}
              onClick={() => sync.mutate()}
              title="Re-read the drop-folder through Tunarr's scan"
            >
              <RefreshCw aria-hidden />
              {sync.isPending ? "Syncing…" : "Sync"}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={aiTag.isPending || !features?.suggestions}
              onClick={() => aiTag.mutate()}
              // AI tagging classifies era/audience/category from text signals (§10) and
              // needs the LLM the Suggest tab uses; without it the button would 409.
              title={
                features?.suggestions
                  ? "Classify untagged commercials with the configured LLM"
                  : "Connect an LLM under Settings → AI to auto-tag clips"
              }
            >
              <Sparkles aria-hidden />
              {aiTag.isPending ? "Tagging…" : "AI tag"}
            </Button>
          </div>
        )}
      </div>

      {sync.error != null && <ErrorState error={sync.error} />}
      {aiTag.error != null && <ErrorState error={aiTag.error} />}
      {clips.error != null && <ErrorState error={clips.error} onRetry={() => clips.refetch()} />}

      {sync.data?.status === 200 && (
        <p className="text-muted-foreground text-sm">
          {`Synced: ${sync.data.data.added} added, ${sync.data.data.updated} updated, ${sync.data.data.pruned} pruned.`}
        </p>
      )}
      {aiTag.data?.status === 200 && (
        <p className="text-muted-foreground text-sm">
          {`Tagged ${aiTag.data.data.tagged} of ${aiTag.data.data.considered} considered${
            aiTag.data.data.partial ? `, ${aiTag.data.data.partial} partially` : ""
          }.`}
        </p>
      )}

      <Card className="flex flex-wrap items-end gap-3 p-4">
        <div className="min-w-48 flex-1">
          <Label htmlFor="clip-search">Search</Label>
          <Input id="clip-search" value={q} placeholder="Clip name" onChange={(e) => setQ(e.target.value)} />
        </div>
        <div>
          <Label htmlFor="clip-kind">Kind</Label>
          <Select id="clip-kind" value={kind} onChange={(e) => setKind(e.target.value)}>
            <option value="">Any</option>
            <option value="commercial">Commercial</option>
            <option value="bumper">Bumper</option>
            <option value="station_id">Station ID</option>
            <option value="psa">PSA</option>
            <option value="trailer">Trailer</option>
            <option value="interstitial">Interstitial</option>
          </Select>
        </div>
        <div>
          <Label htmlFor="clip-audience">Audience</Label>
          <Select id="clip-audience" value={audience} onChange={(e) => setAudience(e.target.value)}>
            <option value="">Any</option>
            <option value="kids">Kids</option>
            <option value="family">Family</option>
            <option value="general">General</option>
            <option value="late_night">Late night</option>
          </Select>
        </div>
        <Button
          variant={untagged ? "default" : "outline"}
          size="sm"
          onClick={() => setUntagged((v) => !v)}
          // "Untagged" means a COMMERCIAL missing a match tag — bumpers do their job
          // without era/audience, so they are never counted as needing work (§10).
          title="Commercials missing era, audience, or category"
        >
          Needs tags
        </Button>
      </Card>

      {rows === undefined ? (
        <p className="text-muted-foreground text-sm">Loading clips…</p>
      ) : rows.length === 0 ? (
        <EmptyState
          title={filtered ? "No clips match" : "No clips yet"}
          description={
            filtered
              ? "Try a wider filter, or clear the search."
              : "Drop files into the filler folder, then Sync. Tunarr scans the folder and assigns each clip its duration."
          }
          {...(filtered
            ? {
                action: {
                  label: "Clear filters",
                  onClick: () => {
                    setQ("");
                    setKind("");
                    setAudience("");
                    setUntagged(false);
                  },
                },
              }
            : {})}
        />
      ) : (
        <>
          <p className="text-muted-foreground text-sm">{pluralize(rows.length, "clip")}</p>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {rows.map((clip) => (
              <ClipCard
                key={clip.tunarrProgramId}
                clip={clip}
                {...(isAdmin ? { onTag: () => setTagging(clip.tunarrProgramId) } : {})}
                {...(isAdmin && clip.aiTagged
                  ? { onConfirmTags: () => setTagging(clip.tunarrProgramId) }
                  : {})}
              />
            ))}
          </div>
        </>
      )}

      {tagging && rows && (
        <ClipTagDialog
          clip={rows.find((c) => c.tunarrProgramId === tagging)}
          onClose={() => setTagging(undefined)}
          onSaved={() => {
            setTagging(undefined);
            void invalidate();
          }}
        />
      )}

      {isAdmin && <IngestPanel ingestAvailable={Boolean(features?.ingest)} onIngested={invalidate} />}
    </div>
  );
};

const PageHeading = () => (
  <div>
    <h1 className="font-semibold text-xl">Filler</h1>
    <p className="mt-1 text-muted-foreground text-sm">
      The commercials, bumpers, and station IDs that play between programs. Tags are what let the scheduler
      match a clip to a channel.
    </p>
  </div>
);

export { FillerPage };
