import * as discoveryApi from "@loomarr/api/endpoints/discovery";
import * as searchApi from "@loomarr/api/endpoints/search";
import type { ChannelIdeaDTO } from "@loomarr/api/models/channelIdeaDTO";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import type { SearchCandidate } from "@loomarr/api/models/searchCandidate";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { provisionKey } from "@loomarr/core/provision";
import { CHANNEL_TEMPLATES } from "@loomarr/core/templates";
import { Plus, Sparkles, X } from "lucide-react";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ideaReason } from "@/home/channel-ideas/channel-ideas";
import { cn } from "@/lib/utils";
import { SearchCommand } from "../../components/loomarr/shell";
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
    <Badge variant="lock">In your library</Badge>
  ) : (
    <Badge variant="tune">Will be added</Badge>
  );

// ManualChannelBuilder — the manual/AI-off channel creation path (#1817, decision G2; approved
// mock #1872). Starter templates fill a name/description only (no template→facet mapping, per
// the maintainer); library facets (from the existing GET /v1/discovery/ideas, #1665/#1720) and
// direct library/TMDB search feed ONE lineup. "Continue to review" is this component's only
// write: POST /v1/discovery/manual-channel re-grounds the picks and returns a job id, which the
// caller hands to the EXISTING ChannelSuggestPanel/ProposalReview — the same submit → review →
// approve/deny gate the AI path uses. Nothing here creates a channel by itself.
const ManualChannelBuilder = ({
  isAdmin,
  initialIdea,
  onCancel,
  onSubmitted,
  className,
}: ManualChannelBuilderProps) => {
  const fromIdea = initialIdea !== undefined;
  const [name, setName] = useState(initialIdea?.name ?? "");
  const [description, setDescription] = useState(initialIdea?.pitch ?? "");
  const [templateId, setTemplateId] = useState<string>();
  const [lineup, setLineup] = useState<ProposalItem[]>(initialIdea?.titles ?? []);
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

  const submit = discoveryApi.useSubmitManualChannel({
    mutation: {
      onSuccess: (res) => {
        if (res.status === 202) onSubmitted(res.data.jobId);
      },
    },
  });

  const lineupKeys = new Set(lineup.map(provisionKey).filter((key) => key !== ""));

  const applyTemplate = (id: string) => {
    const template = CHANNEL_TEMPLATES.find((t) => t.id === id);
    if (!template) return;
    if (templateId === id) {
      setTemplateId(undefined);
      return;
    }
    setTemplateId(id);
    setName(template.label);
    setDescription(template.description);
  };

  const addItem = (item: ProposalItem) => {
    const key = provisionKey(item);
    if (key !== "" && lineupKeys.has(key)) return;
    setLineup((prev) => [...prev, item]);
  };

  const removeItem = (key: string) => setLineup((prev) => prev.filter((item) => provisionKey(item) !== key));

  const canContinue = lineup.length > 0 && name.trim() !== "";
  const continueToReview = () => {
    if (!canContinue) return;
    submit.mutate({ data: { name: name.trim(), description: description.trim(), lineup } });
  };

  const missing = lineup.filter((item) => !item.inLibrary).length;
  const summary =
    lineup.length === 0
      ? "Choose at least one title"
      : missing === 0
        ? `${lineup.length} ${lineup.length === 1 ? "title" : "titles"} · all in your library`
        : `${lineup.length} ${lineup.length === 1 ? "title" : "titles"} · Loomarr will add ${missing} missing ${missing === 1 ? "title" : "titles"}`;

  return (
    <div className={cn("flex flex-col gap-5", className)}>
      <div>
        <h2 className="font-medium text-lg">
          {fromIdea ? "Edit before you request" : isAdmin ? "Add a channel" : "Request a channel"}
        </h2>
        <p className="mt-1 text-muted-foreground text-sm">
          {fromIdea
            ? `Starting from the "${initialIdea.name}" idea. Everything below is already library-grounded — edit the lineup before requesting.`
            : "No AI service is connected yet, so Loomarr can't generate suggestions. Start from a template, build from your library below, or search for titles directly — you'll choose every title before anything is created."}
        </p>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="manual-channel-name">Channel name</Label>
        <Input
          id="manual-channel-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Name this channel"
        />
      </div>

      {!fromIdea && (
        <div>
          <p className="font-medium text-sm">Starter templates</p>
          <ul className="mt-2 flex flex-col gap-2">
            {CHANNEL_TEMPLATES.map((template) => (
              <li key={template.id}>
                <button
                  type="button"
                  onClick={() => applyTemplate(template.id)}
                  className={cn(
                    "flex w-full items-start gap-3 rounded-md border border-border bg-card px-3 py-2.5 text-left transition-colors hover:border-suggest",
                    templateId === template.id && "border-suggest bg-suggest-tint-15",
                  )}
                >
                  <Sparkles className="mt-0.5 size-4 shrink-0 text-suggest" aria-hidden />
                  <span className="min-w-0">
                    <span className="block font-medium text-sm">{template.label}</span>
                    <span className="block text-muted-foreground text-sm">{template.description}</span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}

      {!fromIdea && (
        <div>
          <p className="font-medium text-sm">Build from your library</p>
          <p className="mt-1 text-muted-foreground text-xs">
            Grounded in your library's genres, decades and the seasonal calendar — no AI involved.
          </p>
          <div className="mt-2 flex flex-wrap gap-2">
            {facets.map((idea: ChannelIdeaDTO) => (
              <button
                key={idea.id}
                type="button"
                onClick={() => setActiveFacetId((current) => (current === idea.id ? undefined : idea.id))}
                className={cn(
                  "flex min-w-[150px] flex-col items-start gap-0.5 rounded-md border border-border bg-card px-3 py-2 text-left transition-colors hover:border-tune",
                  activeFacetId === idea.id && "border-tune bg-tune-tint-15",
                )}
              >
                <span className="font-medium text-sm">{idea.name}</span>
                <span className="text-muted-foreground text-xs">{ideaReason(idea, false, Date.now())}</span>
              </button>
            ))}
          </div>
          {activeFacet && (
            <ul className="mt-2 rounded-md border border-border">
              {activeFacet.titles
                .filter((item) => {
                  const key = provisionKey(item);
                  return key === "" || !lineupKeys.has(key);
                })
                .map((item, index) => (
                  <li
                    key={provisionKey(item) || `${activeFacet.id}-${index}`}
                    className="flex items-center gap-2 border-border border-b px-3 py-2 last:border-b-0"
                  >
                    <span className="min-w-0 flex-1 text-sm">
                      {item.name} <span className="text-muted-foreground">{item.year}</span>
                    </span>
                    {lineupBadge(item)}
                    <Button variant="outline" size="sm" onClick={() => addItem(item)}>
                      <Plus aria-hidden /> Add
                    </Button>
                  </li>
                ))}
              {activeFacet.titles.length === 0 && (
                <li className="px-3 py-2 text-muted-foreground text-sm">
                  Every title here is already in your lineup.
                </li>
              )}
            </ul>
          )}
        </div>
      )}

      <div>
        <p className="font-medium text-sm">Search your library or request new titles</p>
        {adding ? (
          <div className="mt-2 flex flex-col gap-2">
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
          </div>
        ) : (
          <Button variant="outline" size="sm" className="mt-2" onClick={() => setAdding(true)}>
            Search titles
          </Button>
        )}
      </div>

      <div>
        <div className="flex items-center justify-between">
          <p className="font-medium text-sm">Your lineup</p>
          {lineup.length > 0 && (
            <Button variant="ghost" size="sm" onClick={() => setLineup([])}>
              Clear
            </Button>
          )}
        </div>
        <ul className="mt-2 rounded-md border border-border">
          {lineup.length === 0 ? (
            <li className="px-3 py-5 text-center text-muted-foreground text-sm">
              No titles yet. Pick a facet above or search for one.
            </li>
          ) : (
            lineup.map((item, index) => {
              const key = provisionKey(item) || `unkeyed-${index}`;
              return (
                <li
                  key={key}
                  className="flex items-center gap-2 border-border border-b px-3 py-2 last:border-b-0"
                >
                  <span className="min-w-0 flex-1 text-sm">
                    {item.name} <span className="text-muted-foreground">{item.year}</span>
                  </span>
                  {lineupBadge(item)}
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label={`Remove ${item.name}`}
                    onClick={() => removeItem(provisionKey(item))}
                  >
                    <X aria-hidden />
                  </Button>
                </li>
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

      <div className="flex flex-wrap items-center justify-between gap-2 border-border border-t pt-4">
        <span className="text-muted-foreground text-sm">{summary}</span>
        <div className="flex gap-2">
          <Button variant="ghost" onClick={onCancel}>
            Cancel
          </Button>
          <Button disabled={!canContinue || submit.isPending} onClick={continueToReview}>
            Continue to review
          </Button>
        </div>
      </div>
    </div>
  );
};

export { ManualChannelBuilder };
