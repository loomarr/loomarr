import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { TaxonDTO } from "@loomarr/api/models/taxonDTO";
import type { TaxonomyImpactCommandDTO } from "@loomarr/api/models/taxonomyImpactCommandDTO";
import type { TaxonomyImpactDTO } from "@loomarr/api/models/taxonomyImpactDTO";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ChevronRight, Plus } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { cn } from "@/lib/utils";

const AXES = ["product", "format", "seasonal", "audience-cue"] as const;
type Axis = (typeof AXES)[number];

const AXIS_COPY: Record<Axis, { label: string; help: string; example: string }> = {
  product: {
    label: "Products & topics",
    help: "What the clip is about or advertising.",
    example: "For example: cereal, cars, local businesses",
  },
  format: {
    label: "Format",
    help: "Descriptive browsing tags. Clip kind separately controls how Loomarr may play it.",
    example: "For example: commercial, promo, station ident",
  },
  seasonal: {
    label: "Seasonal",
    help: "A holiday or time-of-year cue.",
    example: "For example: Christmas, Halloween, back to school",
  },
  "audience-cue": {
    label: "Audience cues",
    help: "Signals useful for matching a channel's audience.",
    example: "For example: kids-oriented, family, late night",
  },
};

type Editor = { mode: "create" | "edit"; axis: Axis; taxon?: TaxonDTO };
type Review = { action: "save" | "delete"; command: TaxonomyImpactCommandDTO; impact: TaxonomyImpactDTO };

const splitTerms = (value: string) =>
  value
    .split(",")
    .map((term) => term.trim())
    .filter(Boolean);

const descendantsOf = (taxa: TaxonDTO[], slug: string): Set<string> => {
  const out = new Set<string>();
  let changed = true;
  while (changed) {
    changed = false;
    for (const taxon of taxa) {
      if (taxon.parent && (taxon.parent === slug || out.has(taxon.parent)) && !out.has(taxon.slug)) {
        out.add(taxon.slug);
        changed = true;
      }
    }
  }
  return out;
};

type TaxonTreeNode = { taxon: TaxonDTO; children: TaxonTreeNode[] };

const treeForAxis = (taxa: TaxonDTO[], axis: Axis): TaxonTreeNode[] => {
  const nodes = taxa.filter((taxon) => taxon.axis === axis);
  const slugs = new Set(nodes.map((taxon) => taxon.slug));
  const children = new Map<string, TaxonDTO[]>();
  for (const taxon of nodes) {
    const parent = taxon.parent && slugs.has(taxon.parent) ? taxon.parent : "";
    children.set(parent, [...(children.get(parent) ?? []), taxon]);
  }
  for (const rows of children.values()) rows.sort((a, b) => a.label.localeCompare(b.label));
  const build = (parent: string): TaxonTreeNode[] =>
    (children.get(parent) ?? []).map((taxon) => ({ taxon, children: build(taxon.slug) }));
  return build("");
};

const TaxonTree = ({
  nodes,
  axis,
  isAdmin,
  onEdit,
  nested = false,
}: {
  nodes: TaxonTreeNode[];
  axis: Axis;
  isAdmin: boolean;
  onEdit: (taxon: TaxonDTO) => void;
  nested?: boolean;
}) => (
  <ul
    className={nested ? "ml-5 border-border border-l" : "divide-y divide-border"}
    {...(!nested ? { "aria-label": `${AXIS_COPY[axis].label} vocabulary` } : {})}
  >
    {nodes.map(({ taxon, children }) => {
      const synonyms = taxon.synonyms ?? [];
      const aliases = taxon.retiredAliases ?? [];
      const resolverTermCount = synonyms.length + aliases.length;
      return (
        <li key={taxon.slug}>
          <div className="flex items-start gap-2 px-4 py-3">
            {nested ? <ChevronRight className="mt-1 size-3 shrink-0 text-static-500" aria-hidden /> : null}
            <div className="min-w-0 flex-1">
              <button
                type="button"
                disabled={!isAdmin}
                className={cn(
                  "break-words text-left font-medium text-sm",
                  isAdmin && "cursor-pointer hover:text-signal",
                )}
                onClick={() => onEdit(taxon)}
              >
                {taxon.label}
              </button>
              <div className="mt-0.5 flex flex-wrap items-center gap-2">
                <span className="break-all font-mono text-muted-foreground text-xs">{taxon.slug}</span>
                {resolverTermCount > 0 ? (
                  <details className="text-muted-foreground text-xs">
                    <summary className="cursor-pointer">
                      {resolverTermCount} classifier {resolverTermCount === 1 ? "term" : "terms"}
                    </summary>
                    <div className="mt-1 max-w-sm space-y-1 break-words">
                      {synonyms.length > 0 ? <p>Synonyms: {synonyms.join(", ")}</p> : null}
                      {aliases.length > 0 ? <p>Retired slugs: {aliases.join(", ")}</p> : null}
                    </div>
                  </details>
                ) : null}
              </div>
            </div>
            <div className="flex shrink-0 flex-wrap items-center justify-end gap-1.5">
              {(taxon.assertedClips ?? 0) > 0 && taxon.assertedClips !== taxon.matchedClips ? (
                <Badge variant="neutral" title="Clips directly assigned this tag">
                  {taxon.assertedClips} direct
                </Badge>
              ) : null}
              <Link
                to="/filler/library"
                search={{ taxon: taxon.slug }}
                className="rounded-sm font-mono text-signal text-xs underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                title={`Browse clips matching ${taxon.label}, including descendants`}
              >
                {taxon.matchedClips.toLocaleString()} {taxon.matchedClips === 1 ? "clip" : "clips"}
              </Link>
            </div>
          </div>
          {children.length > 0 ? (
            <TaxonTree nodes={children} axis={axis} isAdmin={isAdmin} onEdit={onEdit} nested />
          ) : null}
        </li>
      );
    })}
  </ul>
);

const TaxonEditor = ({
  editor,
  taxa,
  onClose,
}: {
  editor: Editor;
  taxa: TaxonDTO[];
  onClose: () => void;
}) => {
  const queryClient = useQueryClient();
  const current = editor.taxon;
  const [slug, setSlug] = useState(current?.slug ?? "");
  const [label, setLabel] = useState(current?.label ?? "");
  const [axis, setAxis] = useState<Axis>(editor.axis);
  const [parent, setParent] = useState(current?.parent ?? "");
  const [synonyms, setSynonyms] = useState((current?.synonyms ?? []).join(", "));
  const [aliases, setAliases] = useState((current?.retiredAliases ?? []).join(", "));
  const [review, setReview] = useState<Review>();

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: fillerApi.getListTaxonomyQueryKey() });
    onClose();
  };
  const fail = (error: unknown) => {
    const problem = toProblem(error);
    toast.error(problem?.title ?? "Couldn't update the taxonomy", {
      ...(problem?.detail ? { description: problem.detail } : {}),
    });
  };
  const create = fillerApi.useCreateTaxon({
    mutation: { onSuccess: () => void refresh(), onError: fail },
  });
  const update = fillerApi.useUpdateTaxon({
    mutation: { onSuccess: () => void refresh(), onError: fail },
  });
  const remove = fillerApi.useDeleteTaxon({
    mutation: {
      onSuccess: () => {
        toast.success("Tag removed", { description: "Its children were kept and moved up one level." });
        void refresh();
      },
      onError: fail,
    },
  });
  const preview = fillerApi.usePreviewTaxonomyEdit();
  const excluded = current ? descendantsOf(taxa, current.slug) : new Set<string>();
  const nodeHasDescendants = excluded.size > 0;
  if (current) excluded.add(current.slug);
  const parentOptions = taxa
    .filter((taxon) => taxon.axis === axis && !excluded.has(taxon.slug))
    .sort((a, b) => a.label.localeCompare(b.label));
  const busy = create.isPending || update.isPending || remove.isPending || preview.isPending;

  const saveCommand = (): TaxonomyImpactCommandDTO => ({
    operation: editor.mode === "create" ? "create" : "update",
    slug: editor.mode === "create" ? slug.trim() : (current?.slug ?? ""),
    label: label.trim(),
    axis,
    ...(parent ? { parent } : {}),
    synonyms: splitTerms(synonyms),
    retiredAliases: splitTerms(aliases),
  });

  const requestReview = (action: Review["action"], command: TaxonomyImpactCommandDTO) => {
    preview.mutate(
      { data: command },
      {
        onSuccess: (response) => {
          if (response.status === 200) setReview({ action, command, impact: response.data });
        },
        onError: fail,
      },
    );
  };

  const applyReview = () => {
    if (!review) return;
    if (review.action === "delete") {
      remove.mutate({ slug: review.command.slug });
      return;
    }
    const command = review.command;
    const common = {
      label: command.label ?? "",
      axis: (command.axis ?? axis) as Axis,
      ...(command.parent ? { parent: command.parent } : {}),
      synonyms: command.synonyms ?? [],
      retiredAliases: command.retiredAliases ?? [],
    };
    if (command.operation === "create") {
      create.mutate({ data: { slug: command.slug, ...common } });
    } else {
      update.mutate({ slug: command.slug, data: common });
    }
  };

  const edit = (change: () => void) => {
    setReview(undefined);
    change();
  };

  return (
    <Card
      role="region"
      className="flex flex-col gap-4 p-4"
      aria-label={current ? `Edit ${current.label}` : "Add tag"}
    >
      <div>
        <h3 className="font-medium">{current ? `Edit ${current.label}` : "Add a tag"}</h3>
        <p className="mt-1 text-muted-foreground text-sm">
          Slugs are what classifiers and saved clips use. Labels are what people see.
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <Label htmlFor="taxon-label">Label</Label>
          <Input
            id="taxon-label"
            value={label}
            onChange={(event) => edit(() => setLabel(event.target.value))}
          />
        </div>
        <div>
          <Label htmlFor="taxon-slug">Slug</Label>
          <Input
            id="taxon-slug"
            value={slug}
            disabled={editor.mode === "edit"}
            placeholder="breakfast-cereal"
            onChange={(event) =>
              edit(() => setSlug(event.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, "-")))
            }
          />
        </div>
        <div>
          <Label htmlFor="taxon-axis">Axis</Label>
          <Select
            value={axis}
            disabled={Boolean(current && nodeHasDescendants)}
            onValueChange={(value) => {
              edit(() => {
                setAxis(value as Axis);
                setParent("");
              });
            }}
          >
            <SelectTrigger id="taxon-axis">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {AXES.map((value) => (
                <SelectItem key={value} value={value}>
                  {AXIS_COPY[value].label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {current && nodeHasDescendants ? (
            <p className="mt-1 text-muted-foreground text-xs">
              Move this tag&rsquo;s children before changing its axis.
            </p>
          ) : null}
        </div>
        <div>
          <Label htmlFor="taxon-parent">Parent</Label>
          <Select
            value={parent || "root"}
            onValueChange={(value) => edit(() => setParent(value === "root" ? "" : value))}
          >
            <SelectTrigger id="taxon-parent">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="root">Top level</SelectItem>
              {parentOptions.map((taxon) => (
                <SelectItem key={taxon.slug} value={taxon.slug}>
                  {taxon.label} ({taxon.slug})
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div>
          <Label htmlFor="taxon-synonyms">Classifier synonyms</Label>
          <Input
            id="taxon-synonyms"
            value={synonyms}
            placeholder="soda, soft drink"
            onChange={(event) => edit(() => setSynonyms(event.target.value))}
          />
        </div>
        <div>
          <Label htmlFor="taxon-aliases">Retired slugs</Label>
          <Input
            id="taxon-aliases"
            value={aliases}
            placeholder="old-machine-name"
            onChange={(event) => edit(() => setAliases(event.target.value))}
          />
        </div>
      </div>
      <p className="text-muted-foreground text-xs">
        Synonyms help the classifier resolve ordinary wording. Retired slugs preserve old integrations; both
        must be unique across the whole vocabulary.
      </p>
      {review ? (
        <Card
          className="space-y-3 border-signal/40 bg-signal/5 p-4"
          aria-label="Change impact"
          aria-live="polite"
        >
          <div>
            <h4 className="font-medium">Review the impact</h4>
            <p className="mt-1 text-muted-foreground text-sm">
              {review.impact.affectedStoredClips > 0
                ? `${review.impact.affectedStoredClips.toLocaleString()} stored clips may have different inherited classification (${review.impact.directStoredClips.toLocaleString()} direct, ${review.impact.descendantStoredClips.toLocaleString()} through descendants).`
                : "No stored clip classifications will change."}
            </p>
            <p className="mt-1 text-muted-foreground text-sm">
              {review.impact.affectedPlayableClips > 0
                ? `${review.impact.affectedPlayableClips.toLocaleString()} playable clips will be checked against channel filler rules after this change.`
                : "No playable clips need channel eligibility checks."}
            </p>
          </div>
          {review.impact.descendants.length > 0 ? (
            <p className="text-sm">
              Descendants kept: {review.impact.descendants.map((node) => node.label).join(", ")}
            </p>
          ) : null}
          {review.impact.savedChannelSelections.length > 0 ? (
            <div className="text-sm">
              <p className="font-medium">Saved channel selections that reference this branch</p>
              <ul className="mt-1 list-disc pl-5">
                {review.impact.savedChannelSelections.map((channel) => (
                  <li key={channel.id}>
                    Channel {channel.number}: {channel.name}
                  </li>
                ))}
              </ul>
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">
              No saved channel selections reference this branch.
            </p>
          )}
          {review.impact.resolverTermsAdded.length > 0 || review.impact.resolverTermsRemoved.length > 0 ? (
            <div className="text-sm">
              <p className="font-medium">Classifier wording</p>
              {review.impact.resolverTermsAdded.length > 0 ? (
                <p className="mt-1">Starts resolving: {review.impact.resolverTermsAdded.join(", ")}</p>
              ) : null}
              {review.impact.resolverTermsRemoved.length > 0 ? (
                <p className="mt-1">Stops resolving: {review.impact.resolverTermsRemoved.join(", ")}</p>
              ) : null}
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">Classifier wording is unchanged.</p>
          )}
          {review.impact.deleteBlocked ? (
            <p className="text-caution text-sm">
              Retag {review.impact.directStoredClips.toLocaleString()} directly assigned stored clips before
              removing this tag. This includes clips in Incoming, removed clips, and compilations.
            </p>
          ) : null}
        </Card>
      ) : null}
      <div className="flex flex-wrap items-center gap-2">
        {current ? (
          <Button
            variant="ghost"
            size="sm"
            disabled={busy}
            onClick={() => requestReview("delete", { operation: "delete", slug: current.slug })}
          >
            Review removal
          </Button>
        ) : (
          <span className="mr-auto" />
        )}
        {review ? (
          <>
            <Button variant="outline" size="sm" disabled={busy} onClick={() => setReview(undefined)}>
              Back
            </Button>
            <Button
              variant={review.action === "delete" ? "destructive" : "default"}
              size="sm"
              disabled={busy || review.impact.deleteBlocked}
              onClick={applyReview}
            >
              {busy
                ? "Applying…"
                : review.action === "delete"
                  ? "Confirm removal"
                  : editor.mode === "create"
                    ? "Confirm add"
                    : "Confirm update"}
            </Button>
          </>
        ) : (
          <>
            <Button variant="outline" size="sm" disabled={busy} onClick={onClose}>
              Cancel
            </Button>
            <Button
              size="sm"
              disabled={busy || !label.trim() || (editor.mode === "create" && !slug.trim())}
              onClick={() => requestReview("save", saveCommand())}
            >
              {busy ? "Checking impact…" : "Review changes"}
            </Button>
          </>
        )}
      </div>
    </Card>
  );
};

const TaxonomyTab = ({ isAdmin }: { isAdmin: boolean }) => {
  const query = fillerApi.useListTaxonomy();
  const data = unwrap(query.data, (body) => body);
  const taxa = data?.taxa ?? [];
  const [editor, setEditor] = useState<Editor>();
  const byAxis = useMemo(
    () =>
      Object.fromEntries(AXES.map((axis) => [axis, treeForAxis(taxa, axis)])) as Record<
        Axis,
        TaxonTreeNode[]
      >,
    [taxa],
  );

  if (query.error) return <ErrorState error={query.error} onRetry={() => query.refetch()} />;
  if (!data) return <p className="text-muted-foreground text-sm">Loading the taxonomy…</p>;

  return (
    <div className="flex flex-col gap-6">
      <div className="grid gap-3 sm:grid-cols-3">
        <Card className="p-4">
          <p className="text-muted-foreground text-xs uppercase tracking-wide">Library coverage</p>
          <p className="mt-1 font-semibold text-2xl">
            {data.taggedClips.toLocaleString()} / {data.totalClips.toLocaleString()}
          </p>
          <p className="mt-1 text-muted-foreground text-sm">
            playable clips have at least one classification signal
          </p>
        </Card>
        <Card className="p-4 sm:col-span-2">
          <p className="font-medium">What belongs here</p>
          <p className="mt-1 text-muted-foreground text-sm">
            Classification describes what a clip contains. Kind is the closed playout role; format signals are
            optional browsing vocabulary. Era, audience, and grounded brand remain separate facts too.
          </p>
          {data.unclassifiedClips > 0 ? (
            <Link
              to="/filler/library"
              search={{ unclassified: true }}
              className="mt-2 inline-flex text-signal text-sm underline-offset-2 hover:underline"
            >
              Review {data.unclassifiedClips.toLocaleString()} unclassified{" "}
              {data.unclassifiedClips === 1 ? "clip" : "clips"}
            </Link>
          ) : (
            <p className="mt-2 text-lock text-sm">Every playable clip has at least one taxonomy tag.</p>
          )}
        </Card>
      </div>

      {editor ? (
        <TaxonEditor
          key={`${editor.mode}:${editor.taxon?.slug ?? editor.axis}`}
          editor={editor}
          taxa={taxa}
          onClose={() => setEditor(undefined)}
        />
      ) : null}

      <section aria-labelledby="classification-coverage-heading">
        <div>
          <h2 id="classification-coverage-heading" className="font-semibold text-lg">
            Classification coverage
          </h2>
          <p className="mt-1 text-muted-foreground text-sm">
            These are independent signals. Missing seasonal or audience cues can be perfectly normal.
          </p>
        </div>
        <div className="mt-3 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          {AXES.map((axis) => {
            const coverage = data.axisCoverage?.find((item) => item.axis === axis);
            return (
              <Card key={axis} className="p-4">
                <h3 className="font-medium">{AXIS_COPY[axis].label}</h3>
                <p className="mt-1 text-muted-foreground text-sm">{AXIS_COPY[axis].help}</p>
                <p className="mt-2 text-muted-foreground text-xs">{AXIS_COPY[axis].example}</p>
                {coverage ? (
                  <div className="mt-3 border-border border-t pt-3 text-sm">
                    <p>
                      <span className="font-medium">{coverage.taggedClips.toLocaleString()}</span> of{" "}
                      {data.totalClips.toLocaleString()} playable clips
                    </p>
                    {coverage.untaggedClips > 0 ? (
                      <Link
                        to="/filler/library"
                        search={{ withoutAxis: axis }}
                        className="mt-1 inline-flex text-signal text-xs underline-offset-2 hover:underline"
                      >
                        Browse {coverage.untaggedClips.toLocaleString()} without this signal
                      </Link>
                    ) : (
                      <p className="mt-1 text-lock text-xs">All playable clips have this signal.</p>
                    )}
                  </div>
                ) : null}
              </Card>
            );
          })}
        </div>
      </section>

      <details className="rounded-lg border border-border bg-panel">
        <summary className="cursor-pointer px-4 py-3 font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
          {isAdmin ? "Manage vocabulary" : "Browse vocabulary"}
        </summary>
        <div className="border-border border-t p-4">
          <p className="mb-4 text-muted-foreground text-sm">
            {isAdmin
              ? "Advanced: change the hierarchy and the words classifiers resolve. Review the impact before any edit is applied."
              : "This hierarchy is read-only. Administrators can change classifier vocabulary and relationships."}
          </p>
          <div className="grid gap-4 xl:grid-cols-2">
            {AXES.map((axis) => {
              return (
                <Card key={axis} className="overflow-hidden">
                  <div className="flex items-start gap-3 border-border border-b p-4">
                    <div className="min-w-0 flex-1">
                      <h3 className="font-medium">{AXIS_COPY[axis].label}</h3>
                      <p className="mt-0.5 text-muted-foreground text-sm">{AXIS_COPY[axis].help}</p>
                      <p className="mt-1 text-muted-foreground text-xs">{AXIS_COPY[axis].example}</p>
                    </div>
                    {isAdmin ? (
                      <Button variant="outline" size="sm" onClick={() => setEditor({ mode: "create", axis })}>
                        <Plus className="size-4" aria-hidden /> Add
                      </Button>
                    ) : null}
                  </div>
                  {byAxis[axis].length === 0 ? (
                    <p className="px-4 py-3 text-muted-foreground text-sm">No terms yet.</p>
                  ) : (
                    <TaxonTree
                      nodes={byAxis[axis]}
                      axis={axis}
                      isAdmin={isAdmin}
                      onEdit={(taxon) => isAdmin && setEditor({ mode: "edit", axis, taxon })}
                    />
                  )}
                </Card>
              );
            })}
          </div>
        </div>
      </details>
    </div>
  );
};

export { TaxonomyTab };
