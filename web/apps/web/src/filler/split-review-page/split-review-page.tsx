import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import { SettingKey } from "@loomarr/api/models/settingKey";
import type { SplitLanguageExclusion } from "@loomarr/api/models/splitLanguageExclusion";
import { toProblem } from "@loomarr/api/mutator";
import { isOk, unwrap } from "@loomarr/api/unwrap";
import { formatMmSs, pluralize } from "@loomarr/core/format";
import { Action, Card, Details, Stack, Text } from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { SplitReviewEditor } from "@/components/loomarr/filler/split-review-editor";
import { languageList, languageName } from "@/lib/languages";
import { useDocumentTitle } from "@/lib/use-document-title";
import type { SplitReviewPageProps } from "./split-review-page.type";

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
    <Card padding={16}>
      <Details
        summary={
          <>
            Loomarr left out {pluralize(exclusions.length, "clip")} spoken in another language
            <Text as="span" marginLeft={4} textRole="compact" tone="secondary">
              · Show skipped {exclusions.length === 1 ? "clip" : "clips"}
            </Text>
          </>
        }
        textRole="cardLabel"
      >
        <Stack marginTop={8}>
          <Text as="p" textRole="compact" tone="secondary">
            This split was checked for {languageList(checkedFor)}. These clips remain in the original
            compilation but won’t be added as filler.
          </Text>
        </Stack>
        <Stack gap="2" marginTop={12} render="ul">
          {exclusions.map((item) => (
            <Stack
              backgroundColor="surfaceCanvas"
              borderRadius={8}
              key={`${item.startMs}-${item.endMs}`}
              paddingHorizontal={12}
              paddingVertical={8}
              render="li"
            >
              <Text as="span" textRole="cardLabel">
                {item.name}
              </Text>
              <Text as="span" textRole="hint" tone="secondary">
                {`${languageName(item.detectedLanguage)} · ${formatMmSs(item.startMs)}–${formatMmSs(item.endMs)}`}
              </Text>
            </Stack>
          ))}
        </Stack>
      </Details>
    </Card>
  );
};

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
  // The route's loader ensured the proposal, and its error view takes a first read that fails, so
  // the page always opens on it. A failed refresh after that is the branch above.
  if (!p) return null;
  const parentName = unwrap(parent.data, (body) => body.clips[0]?.name) || "this compilation";
  const minClipDurationMs = durationSettingMs(
    unwrap(
      settings.data,
      (body) => body.settings.find((entry) => entry.key === SettingKey.FillerMinDuration)?.value,
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
    <Stack gap="4" padding={24}>
      <div>
        {/* This route sits outside the Filler header and tabs, so it names its way back: to
            wherever review was opened (#1817, decided on #1908). The Settings index's "← Manage"
            link is the pattern. */}
        <Stack align="start" marginBottom={12}>
          <Action accessibilityRole="link" icon="goBack" render={<Link to={backTo} />} variant="ghost">
            {from === "incoming" ? "Incoming" : "Library"}
          </Action>
        </Stack>
        <Text as="h1" textRole="title">
          Review clips
        </Text>
        <Stack marginTop={4} maxWidth={672}>
          <Text as="p" textRole="compact" tone="secondary">
            Loomarr found these clips in{" "}
            <Text as="span" textRole="cardLabel" tone="primary">
              {parentName}
            </Text>
            . Preview anything that needs a closer look, then keep the clips you want. The original recording
            stays available if you need it later.
          </Text>
        </Stack>
      </div>
      {languageChanged ? (
        <Card padding={16} tone="caution">
          <Stack
            align={{ base: "stretch", sm: "center" }}
            direction={{ base: "column", sm: "row" }}
            gap="3"
            justify={{ base: "start", sm: "between" }}
          >
            <div>
              <Text as="p" textRole="heading">
                {currentLanguageHeadline}
              </Text>
              <Stack marginTop={4}>
                <Text as="p" textRole="compact" tone="secondary">
                  This split was checked for {languageList(checked)}. Recheck it before you confirm.
                </Text>
              </Stack>
            </div>
            <Action
              disabled={recheck.isPending}
              onPress={() => recheck.mutate({ data: { hash: p.clipHash, from: "split" } })}
              variant="outline"
            >
              {recheck.isPending ? "Starting recheck…" : recheckLabel}
            </Action>
          </Stack>
        </Card>
      ) : null}
      <LanguageReceipt exclusions={p.languageExclusions ?? []} checkedFor={checked} />
      <SplitReviewEditor
        proposal={p}
        {...(minClipDurationMs !== undefined ? { minClipDurationMs } : {})}
        confirming={confirm.isPending}
        onConfirm={(segments) => confirm.mutate({ proposalId, data: { segments } })}
        onBack={() => void navigate({ to: backTo })}
      />
    </Stack>
  );
};

export { SplitReviewPage };
