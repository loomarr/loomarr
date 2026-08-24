import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { TaxonDTO } from "@loomarr/api/models/taxonDTO";
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

const AXIS_COPY: Record<Axis, { label: string; help: string }> = {
  product: { label: "Products & topics", help: "What the clip is about or advertising." },
  format: {
    label: "Format",
    help: "Descriptive browsing tags. Clip kind separately controls how Loomarr may play it.",
  },
  seasonal: { label: "Seasonal", help: "A holiday or time-of-year cue." },
  "audience-cue": { label: "Audience cues", help: "Signals useful for matching a channel's audience." },
};

type Editor = { mode: "create" | "edit"; axis: Axis; taxon?: TaxonDTO };

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

const flattenAxis = (taxa: TaxonDTO[], axis: Axis): Array<{ taxon: TaxonDTO; depth: number }> => {
  const nodes = taxa.filter((taxon) => taxon.axis === axis);
  const slugs = new Set(nodes.map((taxon) => taxon.slug));
  const children = new Map<string, TaxonDTO[]>();
  for (const taxon of nodes) {
    const parent = taxon.parent && slugs.has(taxon.parent) ? taxon.parent : "";
    children.set(parent, [...(children.get(parent) ?? []), taxon]);
  }
  for (const rows of children.values()) rows.sort((a, b) => a.label.localeCompare(b.label));
  const out: Array<{ taxon: TaxonDTO; depth: number }> = [];
  const visit = (parent: string, depth: number) => {
    for (const taxon of children.get(parent) ?? []) {
      out.push({ taxon, depth });
      visit(taxon.slug, depth + 1);
    }
  };
  visit("", 0);
  return out;
};

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
  const [confirmDelete, setConfirmDelete] = useState(false);

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
  const storedAssignments = current?.storedClips ?? current?.assertedClips ?? 0;
  const blocked = storedAssignments > 0;
  const excluded = current ? descendantsOf(taxa, current.slug) : new Set<string>();
  const nodeHasDescendants = excluded.size > 0;
  if (current) excluded.add(current.slug);
  const parentOptions = taxa
    .filter((taxon) => taxon.axis === axis && !excluded.has(taxon.slug))
    .sort((a, b) => a.label.localeCompare(b.label));
  const busy = create.isPending || update.isPending || remove.isPending;

  const save = () => {
    const common = {
      label: label.trim(),
      axis,
      ...(parent ? { parent } : {}),
      synonyms: splitTerms(synonyms),
      retiredAliases: splitTerms(aliases),
    };
    if (editor.mode === "create") {
      create.mutate({ data: { slug: slug.trim(), ...common } });
    } else if (current) {
      update.mutate({ slug: current.slug, data: common });
    }
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
          <Input id="taxon-label" value={label} onChange={(event) => setLabel(event.target.value)} />
        </div>
        <div>
          <Label htmlFor="taxon-slug">Slug</Label>
          <Input
            id="taxon-slug"
            value={slug}
            disabled={editor.mode === "edit"}
            placeholder="breakfast-cereal"
            onChange={(event) => setSlug(event.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, "-"))}
          />
        </div>
        <div>
          <Label htmlFor="taxon-axis">Axis</Label>
          <Select
            value={axis}
            disabled={Boolean(current && nodeHasDescendants)}
            onValueChange={(value) => {
              setAxis(value as Axis);
              setParent("");
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
            onValueChange={(value) => setParent(value === "root" ? "" : value)}
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
            onChange={(event) => setSynonyms(event.target.value)}
          />
        </div>
        <div>
          <Label htmlFor="taxon-aliases">Retired slugs</Label>
          <Input
            id="taxon-aliases"
            value={aliases}
            placeholder="old-machine-name"
            onChange={(event) => setAliases(event.target.value)}
          />
        </div>
      </div>
      <p className="text-muted-foreground text-xs">
        Synonyms help the classifier resolve ordinary wording. Retired slugs preserve old integrations; both
        must be unique across the whole vocabulary.
      </p>
      <div className="flex flex-wrap items-center gap-2">
        {current ? (
          blocked ? (
            <p className="mr-auto text-caution text-xs">
              Retag {storedAssignments} stored {storedAssignments === 1 ? "clip" : "clips"} before removing
              this tag. This count includes clips in Incoming, removed clips, and compilations.
            </p>
          ) : (
            <Button
              variant={confirmDelete ? "destructive" : "ghost"}
              size="sm"
              disabled={busy}
              onClick={() => (confirmDelete ? remove.mutate({ slug: current.slug }) : setConfirmDelete(true))}
            >
              {confirmDelete ? "Confirm remove" : "Remove tag"}
            </Button>
          )
        ) : (
          <span className="mr-auto" />
        )}
        <Button variant="outline" size="sm" disabled={busy} onClick={onClose}>
          Cancel
        </Button>
        <Button
          size="sm"
          disabled={busy || !label.trim() || (editor.mode === "create" && !slug.trim())}
          onClick={save}
        >
          {busy ? "Saving…" : "Save"}
        </Button>
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
      Object.fromEntries(AXES.map((axis) => [axis, flattenAxis(taxa, axis)])) as Record<
        Axis,
        Array<{ taxon: TaxonDTO; depth: number }>
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
          <p className="mt-1 text-muted-foreground text-sm">playable clips have at least one taxonomy tag</p>
        </Card>
        <Card className="p-4 sm:col-span-2">
          <p className="font-medium">What belongs here</p>
          <p className="mt-1 text-muted-foreground text-sm">
            Taxonomy describes what a clip contains. Kind is the closed playout role; format tags are optional
            browsing vocabulary. Era, audience, and grounded brand remain separate facts too.
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

      <div className="grid gap-4 xl:grid-cols-2">
        {AXES.map((axis) => {
          const coverage = data.axisCoverage?.find((item) => item.axis === axis);
          return (
            <Card key={axis} className="overflow-hidden">
              <div className="flex items-start gap-3 border-border border-b p-4">
                <div className="min-w-0 flex-1">
                  <h2 className="font-medium">{AXIS_COPY[axis].label}</h2>
                  <p className="mt-0.5 text-muted-foreground text-sm">{AXIS_COPY[axis].help}</p>
                  {coverage ? (
                    <p className="mt-1 text-muted-foreground text-xs">
                      {coverage.taggedClips.toLocaleString()} {coverage.taggedClips === 1 ? "clip" : "clips"}{" "}
                      tagged
                      {coverage.untaggedClips > 0 ? (
                        <>
                          {" · "}
                          <Link
                            to="/filler/library"
                            search={{ withoutAxis: axis }}
                            className="text-signal underline-offset-2 hover:underline"
                          >
                            Browse {coverage.untaggedClips.toLocaleString()} without
                          </Link>
                        </>
                      ) : null}
                    </p>
                  ) : null}
                </div>
                {isAdmin ? (
                  <Button variant="outline" size="sm" onClick={() => setEditor({ mode: "create", axis })}>
                    <Plus className="size-4" aria-hidden /> Add
                  </Button>
                ) : null}
              </div>
              <ul className="divide-y divide-border">
                {byAxis[axis].map(({ taxon, depth }) => {
                  const synonyms = taxon.synonyms ?? [];
                  const aliases = taxon.retiredAliases ?? [];
                  const resolverTermCount = synonyms.length + aliases.length;
                  return (
                    <li
                      key={taxon.slug}
                      className="flex items-start gap-2 px-4 py-3"
                      style={{ paddingLeft: `${16 + depth * 20}px` }}
                    >
                      {depth > 0 ? (
                        <ChevronRight className="mt-1 size-3 shrink-0 text-static-500" aria-hidden />
                      ) : null}
                      <div className="min-w-0 flex-1">
                        <button
                          type="button"
                          disabled={!isAdmin}
                          className={cn(
                            "text-left font-medium text-sm",
                            isAdmin && "cursor-pointer hover:text-signal",
                          )}
                          onClick={() => isAdmin && setEditor({ mode: "edit", axis, taxon })}
                        >
                          {taxon.label}
                        </button>
                        <div className="mt-0.5 flex flex-wrap items-center gap-2">
                          <span className="font-mono text-muted-foreground text-xs">{taxon.slug}</span>
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
                      <div className="flex shrink-0 items-center gap-1.5">
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
                    </li>
                  );
                })}
              </ul>
            </Card>
          );
        })}
      </div>
    </div>
  );
};

export { TaxonomyTab };
