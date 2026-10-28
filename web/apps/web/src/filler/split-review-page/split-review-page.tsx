import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import type { SplitLanguageExclusion } from "@loomarr/api/models/splitLanguageExclusion";
import { toProblem } from "@loomarr/api/mutator";
import { isOk, unwrap } from "@loomarr/api/unwrap";
import { formatMmSs, pluralize } from "@loomarr/core/format";
import { useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { toast } from "sonner";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { SplitReviewEditor } from "@/components/loomarr/filler/split-review-editor";
import { Button } from "@/components/ui/button";
import { languageList, languageName } from "@/lib/languages";
import { useDocumentTitle } from "@/lib/use-document-title";
import type { SplitReviewOrigin, SplitReviewPageProps } from "./split-review-page.type";

const durationSettingMs = (value: string | undefined): number | undefined => {
  if (!value) return undefined;
  const match = /^(?:(\d+(?:\.\d+)?)h)?(?:(\d+(?:\.\d+)?)m)?(?:(\d+(?:\.\d+)?)s)?$/.exec(value);
  if (!match || (!match[1] && !match[2] && !match[3])) return undefined;
  return (Number(match[1] ?? 0) * 3600 + Number(match[2] ?? 0) * 60 + Number(match[3] ?? 0)) * 1000;
};

const LanguageReceipt = ({
  exclusions,
  checkedFor,
}: {
  exclusions: SplitLanguageExclusion[];
  checkedFor: string[];
}) => {
  if (exclusions.length === 0) return null;
  return (
    <details className="border-border bg-muted/20 rounded-lg border p-4 text-sm">
      <summary className="cursor-pointer font-medium">
        Loomarr left out {pluralize(exclusions.length, "clip")} spoken in another language
        <span className="text-muted-foreground ml-1 font-normal">
          · Show skipped {exclusions.length === 1 ? "clip" : "clips"}
        </span>
      </summary>
      <p className="text-muted-foreground mt-2">
        This split was checked for {languageList(checkedFor)}. These clips remain in the original compilation
        but won’t be added as filler.
      </p>
      <ul className="mt-3 space-y-2">
        {exclusions.map((item) => (
          <li key={`${item.startMs}-${item.endMs}`} className="bg-background/60 rounded-md px-3 py-2">
            <span className="block font-medium">{item.name}</span>
            <span className="text-muted-foreground text-xs">
              {`${languageName(item.detectedLanguage)} · ${formatMmSs(item.startMs)}–${formatMmSs(item.endMs)}`}
            </span>
          </li>
        ))}
      </ul>
    </details>
  );
};

// The route's `from=` search param names where review was opened, so Back can return there. Only
// the two known origins are kept; anything else (a deep link, a hand-edited URL) is dropped and
// Back falls back to Library, where it always went before. Optional, so a link may omit it.
const validateSplitReviewSearch = (search: Record<string, unknown>): { from?: SplitReviewOrigin } =>
  search.from === "incoming" || search.from === "library" ? { from: search.from } : {};

// SplitReviewPage — the /filler/splits/$proposalId screen (§10 V34). Detection persisted
// a proposal; this page reads it back (the GET is the truth — the SSE frame was only the
// doorbell) and hands it to the editor. Confirm commits the operator's edited cut list;
// Back leaves the proposal persisted for later — there is deliberately no reject endpoint.
const SplitReviewPage = ({ proposalId, from }: SplitReviewPageProps) => {
  useDocumentTitle("Review clips");
  const backTo = from === "incoming" ? "/filler/incoming" : "/filler/library";
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  // Admin-only: the route mounts this page inside AdminOnly, so a member never reaches these reads
  // (every split route 403s for a member server-side anyway — §11, §19).
  const proposal = fillerApi.useGetFillerSplit(proposalId);
  const settings = settingsApi.useSettingsList({ query: { retry: false } });
  const persistedProposal = unwrap(proposal.data);
  // Resolve the composite through the existing exact-hash catalog read. Composites are deliberately
  // absent from the airable catalog by default, so omitting includeComposites would turn a valid
  // parent into a miss and tempt this page back to exposing its storage hash.
  const parent = fillerApi.useListFiller(
    {
      hashes: persistedProposal ? [persistedProposal.clipHash] : [],
      includeComposites: true,
      limit: 1,
    },
    { query: { enabled: Boolean(persistedProposal) } },
  );

  const confirm = fillerApi.useConfirmFillerSplit({
    mutation: {
      onSuccess: (res) => {
        // Segments are clips now and the compilation remains as their non-airable parent. The
        // catalog is the surface that changed, so it refetches before the operator lands back on it.
        void queryClient.invalidateQueries({ queryKey: fillerApi.getListFillerQueryKey() });
        const clips = isOk(res) ? res.data.clips : 0;
        toast.success(clips > 0 ? `Split into ${clips} clips` : "Split confirmed");
        void navigate({ to: "/filler/library" });
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't confirm the split"),
    },
  });
  const recheck = fillerApi.useRewindFillerClip({
    mutation: {
      onSuccess: () => {
        void queryClient.invalidateQueries({ queryKey: fillerApi.getFillerIncomingQueryKey() });
        toast.success("Rechecking this compilation");
        void navigate({ to: "/filler/incoming" });
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't start the recheck"),
    },
  });

  if (proposal.error != null) {
    return <ErrorState error={proposal.error} onRetry={() => proposal.refetch()} />;
  }
  const p = persistedProposal;
  if (!p) return <p className="text-muted-foreground text-sm">Loading the proposal…</p>;
  const parentName = unwrap(parent.data, (body) => body.clips[0]?.name) || "this compilation";
  const minClipDurationMs = durationSettingMs(
    unwrap(
      settings.data,
      (body) => body.settings.find((entry) => entry.key === "filler.min_duration")?.value,
    ),
  );
  // What intake admits is the channels' commercial languages (#2030 decision 13), so a new channel's
  // language changes it as surely as the household setting does. The server reports both sets.
  const checked = p.checkedLanguages;
  const current = p.currentLanguages;
  const languageChanged =
    checked.length > 0 &&
    (checked.length !== current.length || checked.some((code) => !current.includes(code)));
  const currentLanguageHeadline =
    current.length === 0
      ? "Commercials can now use any language"
      : current.length === 1
        ? `Your commercial language is now ${languageList(current)}`
        : `Your commercial languages are now ${languageList(current)}`;
  const recheckLabel =
    current.length > 0 ? `Recheck with ${languageList(current)}` : "Recheck without a language filter";

  return (
    // p-6 for the same reason as the catalog page: the shell adds no gutter, so a page
    // without one renders flush against the sidebar.
    <div className="flex flex-col gap-4 p-6">
      <div>
        {/* This route sits outside the Filler header and tabs, so it names its way back: to
            wherever review was opened (#1817, decided on #1908). The Settings index's "← Manage"
            link is the pattern. */}
        <Link
          to={backTo}
          className="text-muted-foreground hover:text-foreground mb-3 inline-flex items-center gap-1.5 text-sm"
        >
          <ArrowLeft className="size-3.5" aria-hidden />
          {from === "incoming" ? "Incoming" : "Library"}
        </Link>
        <h1 className="text-xl font-semibold">Review clips</h1>
        <p className="text-muted-foreground mt-1 max-w-2xl text-sm">
          Loomarr found these clips in <span className="text-static-200 font-medium">{parentName}</span>.
          Preview anything that needs a closer look, then keep the clips you want. The original recording
          stays available if you need it later.
        </p>
      </div>
      {languageChanged ? (
        <div className="border-caution/40 bg-caution-tint-15 flex flex-col gap-3 rounded-lg border p-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p className="font-medium">{currentLanguageHeadline}</p>
            <p className="text-muted-foreground mt-1 text-sm">
              This split was checked for {languageList(checked)}. Recheck it before you confirm.
            </p>
          </div>
          <Button
            size="sm"
            variant="outline"
            disabled={recheck.isPending}
            onClick={() => recheck.mutate({ data: { hash: p.clipHash, from: "split" } })}
          >
            {recheck.isPending ? "Starting recheck…" : recheckLabel}
          </Button>
        </div>
      ) : null}
      <LanguageReceipt exclusions={p.languageExclusions ?? []} checkedFor={checked} />
      <SplitReviewEditor
        proposal={p}
        {...(minClipDurationMs !== undefined ? { minClipDurationMs } : {})}
        confirming={confirm.isPending}
        onConfirm={(segments) => confirm.mutate({ proposalId, data: { segments } })}
        onBack={() => void navigate({ to: backTo })}
      />
    </div>
  );
};

export { SplitReviewPage, validateSplitReviewSearch };
