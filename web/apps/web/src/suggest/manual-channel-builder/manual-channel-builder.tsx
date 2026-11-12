import * as discoveryApi from "@loomarr/api/endpoints/discovery";
import * as searchApi from "@loomarr/api/endpoints/search";
import type { ChannelIdeaDTO } from "@loomarr/api/models/channelIdeaDTO";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import type { SearchCandidate } from "@loomarr/api/models/searchCandidate";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { provisionKey } from "@loomarr/core/provision";
import { ideaReason } from "@loomarr/core/requests";
import { Action, Badge, ChoiceCard, Field, Stack } from "@loomarr/design-system";
import { AiReconnectBanner } from "@loomarr/ui";
import { useStore } from "@tanstack/react-form";
import { useState } from "react";
import { useAppForm } from "@/lib/app-form";
import { cn } from "@/lib/utils";
import { SearchCommand } from "../../components/loomarr/shell";
import { useAiReconnect } from "../use-ai-reconnect";
import type { ManualChannelBuilderProps } from "./manual-channel-builder.type";

const proposalItemFromCandidate = (candidate: SearchCandidate): ProposalItem => ({
  name: candidate.name,
  mediaType: candidate.mediaType,
  inLibrary: candidate.inLibrary,
  ...(candidate.libraryItemId ? { libraryItemId: candidate.libraryItemId } : {}),
  ...(candidate.year ? { year: candidate.year } : {}),
  ...(candidate.tmdbId ? { tmdbId: candidate.tmdbId } : {}),
  ...(candidate.tvdbId ? { tvdbId: candidate.tvdbId } : {}),
  ...(candidate.genres ? { genres: candidate.genres } : {}),
});

const lineupBadge = (item: ProposalItem) =>
  item.inLibrary ? (
    <Badge shape="square" tone="lock">
      In your library
    </Badge>
  ) : (
    <Badge shape="square" tone="tune">
      Will be added
    </Badge>
  );

// ManualChannelBuilder — the manual/AI-off channel creation path (#1817, decision G2; approved
// mock #1872). Library facets (from the existing GET /v1/discovery/ideas, #1665/#1720) and direct
// library/TMDB search feed ONE lineup. It offers no starter templates: they were the AI path's four
// example chips again, and filled only a name and a description that build nothing without AI
// (#2746, #3411). "Continue to review" is this component's only
// write: POST /v1/discovery/manual-channel re-grounds the picks and returns a job id, which the
// caller hands to the EXISTING ChannelSuggestPanel/ProposalReview — the same submit → review →
// approve/deny gate the AI path uses. Nothing here creates a channel by itself.
const ManualChannelBuilder = ({
  isAdmin,
  initialIdea,
  onCancel,
  onSubmitted,
  onAiSubmitted,
  onDescribeWithAi,
  className,
}: ManualChannelBuilderProps) => {
  const fromIdea = initialIdea !== undefined;
  const submit = discoveryApi.useSubmitManualChannel();
  // The name, the description an idea fills in, and the one lineup the picks build.
  const form = useAppForm({
    defaultValues: {
      name: initialIdea?.name ?? "",
      description: initialIdea?.pitch ?? "",
      lineup: initialIdea?.titles ?? ([] as ProposalItem[]),
    },
    onSubmit: ({ value }) => {
      if (value.lineup.length > 0 && value.name.trim() !== "") {
        submitLineup(onSubmitted);
      }
    },
  });
  const { name, description, lineup } = useStore(form.store, (state) => state.values);
  const setLineup = (next: ProposalItem[] | ((prev: ProposalItem[]) => ProposalItem[])) =>
    form.setFieldValue("lineup", next);
  const [activeFacetId, setActiveFacetId] = useState<string>();
  const [adding, setAdding] = useState(false);
  const [query, setQuery] = useState("");

  const ideasList = discoveryApi.useListChannelIdeas({ query: { enabled: !fromIdea } });
  const facets = fromIdea ? [] : (unwrap(ideasList.data, (body) => body.ideas) ?? []);
  const activeFacet = facets.find((idea: ChannelIdeaDTO) => idea.id === activeFacetId);

  const search = searchApi.useSearch(
    { q: query, scope: "all", limit: 8 },
    { query: { enabled: adding && query.trim().length > 1 } },
  );
  const candidates = unwrap(search.data, (body) => body.candidates) ?? [];

  // "Keep building manually" hides the banner for the rest of this builder session, and with it
  // the polling.
  const [keepManual, setKeepManual] = useState(false);
  const offersAi = onAiSubmitted !== undefined && onDescribeWithAi !== undefined;
  const aiReconnected = useAiReconnect(isAdmin && offersAi && !keepManual);

  const lineupKeys = new Set(lineup.map(provisionKey).filter((key) => key !== ""));

  const addItem = (item: ProposalItem) => {
    const key = provisionKey(item);
    if (key !== "" && lineupKeys.has(key)) return;
    setLineup((prev) => [...prev, item]);
  };

  const removeItem = (key: string) => setLineup((prev) => prev.filter((item) => provisionKey(item) !== key));

  const canContinue = lineup.length > 0 && name.trim() !== "";
  const submitLineup = (submitted: (jobId: string) => void) =>
    submit.mutate(
      { data: { name: name.trim(), description: description.trim(), lineup } },
      {
        onSuccess: (res) => {
          if (res.status === 202) submitted(res.data.jobId);
        },
      },
    );
  // The picks go through the same grounded submit as "Continue to review", so AI starts from
  // the recorded lineup rather than from a copy of it. AI is asked about the description the
  // person wrote; the name stands in when there is none. The typed name rides along so the
  // revision keeps it rather than taking the model's.
  const tryAi = () => {
    const brief = description.trim() || name.trim();
    if (lineup.length === 0) {
      onDescribeWithAi?.(brief);
      return;
    }
    if (!canContinue) return;
    submitLineup((jobId) => onAiSubmitted?.(jobId, brief, name.trim()));
  };
  const tryAiBlockedReason =
    lineup.length > 0 && name.trim() === "" ? "Name this channel to try AI suggestions." : undefined;

  const missing = lineup.filter((item) => !item.inLibrary).length;
  const summary =
    lineup.length === 0
      ? "Choose at least one title"
      : missing === 0
        ? `${lineup.length} ${lineup.length === 1 ? "title" : "titles"} · all in your library`
        : `${lineup.length} ${lineup.length === 1 ? "title" : "titles"} · Loomarr will add ${missing} missing ${missing === 1 ? "title" : "titles"}`;

  return (
    <div className={cn("flex flex-col gap-5", className)}>
      {/* The page's one heading names this ("Add a channel", or "Edit before you request" from an
          idea); this sentence is its description, so the builder nests no heading of its own. */}
      <p className="text-muted-foreground text-sm">
        {fromIdea
          ? `Starting from the "${initialIdea.name}" idea. Everything below is already library-grounded — edit the lineup before requesting.`
          : "No AI service is connected yet, so Loomarr can't generate suggestions. Build from your library below, or search for titles directly — you'll choose every title before anything is created."}
      </p>

      {aiReconnected && !keepManual && (
        <AiReconnectBanner
          pickCount={lineup.length}
          busy={submit.isPending}
          tryAiBlockedReason={tryAiBlockedReason}
          onTryAi={tryAi}
          onKeepManual={() => setKeepManual(true)}
        />
      )}

      <form.Field name="name">
        {(field) => (
          <Field
            autoCapitalize="words"
            id="manual-channel-name"
            label="Channel name"
            onChangeText={field.handleChange}
            placeholder="Name this channel"
            value={field.state.value}
          />
        )}
      </form.Field>

      {!fromIdea && (
        <div>
          <p className="text-sm font-medium">Build from your library</p>
          <p className="text-muted-foreground mt-1 text-xs">
            Grounded in your library's genres, decades and the seasonal calendar — no AI involved.
          </p>
          <Stack className="mt-2" direction="row" gap="inline" wrap>
            {facets.map((idea: ChannelIdeaDTO) => (
              <div key={idea.id} className="min-w-[150px]">
                <ChoiceCard
                  accent="info"
                  description={ideaReason(idea, false, Date.now())}
                  onPress={() => setActiveFacetId((current) => (current === idea.id ? undefined : idea.id))}
                  selected={activeFacetId === idea.id}
                  title={idea.name}
                />
              </div>
            ))}
          </Stack>
          {activeFacet && (
            <ul className="border-border mt-2 rounded-md border">
              {activeFacet.titles
                .filter((item) => {
                  const key = provisionKey(item);
                  return key === "" || !lineupKeys.has(key);
                })
                .map((item, index) => (
                  <Stack
                    align="center"
                    className="border-border border-b px-3 py-2 last:border-b-0"
                    direction="row"
                    gap="inline"
                    key={provisionKey(item) || `${activeFacet.id}-${index}`}
                    render="li"
                  >
                    <span className="min-w-0 flex-1 text-sm">
                      {item.name} <span className="text-muted-foreground">{item.year}</span>
                    </span>
                    {lineupBadge(item)}
                    <Action
                      accessibilityLabel={`Add ${item.name}`}
                      icon="plus"
                      onPress={() => addItem(item)}
                      variant="outline"
                    >
                      Add
                    </Action>
                  </Stack>
                ))}
              {activeFacet.titles.length === 0 && (
                <li className="text-muted-foreground px-3 py-2 text-sm">
                  Every title here is already in your lineup.
                </li>
              )}
            </ul>
          )}
        </div>
      )}

      <div>
        <p className="text-sm font-medium">Search your library or request new titles</p>
        {adding ? (
          <Stack className="mt-2" gap="inline">
            <SearchCommand
              query={query}
              onQueryChange={setQuery}
              loading={search.isFetching}
              placeholder="Search for a movie or show…"
              onEscape={() => {
                setQuery("");
                setAdding(false);
              }}
              results={candidates
                .filter((candidate) => {
                  const key = provisionKey(candidate);
                  return key === "" || !lineupKeys.has(key);
                })
                .map((candidate) => ({
                  id: provisionKey(candidate) || candidate.name,
                  scope: candidate.inLibrary ? ("library" as const) : ("tmdb" as const),
                  name: candidate.name,
                  ...(candidate.year ? { meta: String(candidate.year) } : {}),
                  inLibrary: candidate.inLibrary,
                }))}
              onSelect={(result) => {
                const candidate = candidates.find(
                  (value) => provisionKey(value) === result.id || value.name === result.id,
                );
                if (candidate) {
                  addItem(proposalItemFromCandidate(candidate));
                  setQuery("");
                  setAdding(false);
                }
              }}
            />
          </Stack>
        ) : (
          <div className="mt-2">
            <Action onPress={() => setAdding(true)} variant="outline">
              Search titles
            </Action>
          </div>
        )}
      </div>

      <div>
        <Stack align="center" direction="row" justify="between">
          <p className="text-sm font-medium">Your lineup</p>
          {lineup.length > 0 && (
            <Action onPress={() => setLineup([])} variant="ghost">
              Clear
            </Action>
          )}
        </Stack>
        <ul className="border-border mt-2 rounded-md border">
          {lineup.length === 0 ? (
            <li className="text-muted-foreground px-3 py-5 text-center text-sm">
              No titles yet. Pick a facet above or search for one.
            </li>
          ) : (
            lineup.map((item, index) => {
              const key = provisionKey(item) || `unkeyed-${index}`;
              return (
                <Stack
                  align="center"
                  className="border-border border-b px-3 py-2 last:border-b-0"
                  direction="row"
                  gap="inline"
                  key={key}
                  render="li"
                >
                  <span className="min-w-0 flex-1 text-sm">
                    {item.name} <span className="text-muted-foreground">{item.year}</span>
                  </span>
                  {lineupBadge(item)}
                  <Action
                    icon="close"
                    layout="icon"
                    onPress={() => removeItem(provisionKey(item))}
                    variant="ghost"
                  >
                    {`Remove ${item.name}`}
                  </Action>
                </Stack>
              );
            })
          )}
        </ul>
      </div>

      {submit.error != null && (
        <p className="text-onair-300 text-sm">
          {toProblem(submit.error).title ?? "That didn't go through. Try again."}
        </p>
      )}

      <Stack
        align="center"
        className="border-border border-t pt-4"
        direction="row"
        gap="inline"
        justify="between"
        wrap
      >
        <span className="text-muted-foreground text-sm">{summary}</span>
        <Stack direction="row" gap="inline">
          <Action onPress={onCancel} variant="ghost">
            Cancel
          </Action>
          <Action disabled={!canContinue || submit.isPending} onPress={() => void form.handleSubmit()}>
            Continue to review
          </Action>
        </Stack>
      </Stack>
    </div>
  );
};

export { ManualChannelBuilder };
