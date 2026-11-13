import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { TaxonDTO } from "@loomarr/api/models/taxonDTO";
import type { TaxonomyImpactCommandDTO } from "@loomarr/api/models/taxonomyImpactCommandDTO";
import type { TaxonomyImpactDTO } from "@loomarr/api/models/taxonomyImpactDTO";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { Action, Badge, Card, Field, Icon, icons, Stack, Text } from "@loomarr/design-system";
import { useStore } from "@tanstack/react-form";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import { BrowserSelectControl } from "@/client-adapters";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useAppForm } from "@/lib/app-form";
import { TAXONOMY_AXES as AXES, type TaxonomyAxis as Axis } from "../taxonomy-axes";

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
  presentation: {
    label: "Presentation",
    help: "How the pictures are made, separate from what the clip is about or who may watch it.",
    example: "For example: animated, live action",
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
    className={nested ? "border-border ml-5 border-l" : "divide-border divide-y"}
    {...(!nested ? { "aria-label": `${AXIS_COPY[axis].label} vocabulary` } : {})}
  >
    {nodes.map(({ taxon, children }) => {
      const synonyms = taxon.synonyms ?? [];
      const aliases = taxon.retiredAliases ?? [];
      const resolverTermCount = synonyms.length + aliases.length;
      return (
        <li key={taxon.slug}>
          <Stack align="start" direction="row" gap="inline" paddingHorizontal={16} paddingVertical={12}>
            {nested ? (
              <Stack direction="row" marginTop={4} render="span">
                <Icon decorative glyph={icons.rowExpand} size="compact" tone="muted" />
              </Stack>
            ) : null}
            <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minWidth={0}>
              <Action
                disabled={!isAdmin}
                layout="text"
                onPress={() => onEdit(taxon)}
                style={{ alignSelf: "flex-start" }}
                variant="ghost"
              >
                {taxon.label}
              </Action>
              <Stack align="center" direction="row" gap="inline" marginTop={2} wrap>
                <Text as="span" className="break-all" textRole="mono">
                  {taxon.slug}
                </Text>
                {resolverTermCount > 0 ? (
                  <details>
                    <Text as="summary" className="cursor-pointer" textRole="hint" tone="secondary">
                      {resolverTermCount} classifier {resolverTermCount === 1 ? "term" : "terms"}
                    </Text>
                    <Stack className="break-words" gap="1" marginTop={4} maxWidth={384}>
                      {synonyms.length > 0 ? (
                        <Text as="p" textRole="hint" tone="secondary">
                          Synonyms: {synonyms.join(", ")}
                        </Text>
                      ) : null}
                      {aliases.length > 0 ? (
                        <Text as="p" textRole="hint" tone="secondary">
                          Retired slugs: {aliases.join(", ")}
                        </Text>
                      ) : null}
                    </Stack>
                  </details>
                ) : null}
              </Stack>
            </Stack>
            <Stack align="center" direction="row" flexShrink={0} gap="1.5" justify="end" wrap>
              {(taxon.assertedClips ?? 0) > 0 && taxon.assertedClips !== taxon.matchedClips ? (
                <Badge shape="square" title="Clips directly assigned this tag" tone="neutral">
                  {taxon.assertedClips.toLocaleString()} direct
                </Badge>
              ) : null}
              <Link
                to="/filler/library"
                search={{ taxon: taxon.slug }}
                className="focus-visible:ring-ring rounded-sm underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:outline-none"
                title={`Browse clips matching ${taxon.label}, including descendants`}
              >
                <Text as="span" textRole="mono" tone="signal">
                  {taxon.matchedClips.toLocaleString()} {taxon.matchedClips === 1 ? "clip" : "clips"}
                </Text>
              </Link>
            </Stack>
          </Stack>
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
  // The term's fields. Saving never submits the form: it asks for an impact review first, so the
  // buttons below run the review and the form only holds the values.
  const form = useAppForm({
    defaultValues: {
      slug: current?.slug ?? "",
      label: current?.label ?? "",
      axis: editor.axis as Axis,
      parent: current?.parent ?? "",
      synonyms: (current?.synonyms ?? []).join(", "),
      aliases: (current?.retiredAliases ?? []).join(", "),
    },
  });
  const { slug, label, axis, parent, synonyms, aliases } = useStore(form.store, (state) => state.values);
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
    <Card aria-label={current ? `Edit ${current.label}` : "Add tag"} gap={16} padding={16} role="region">
      <Stack gap="1">
        <Text as="h3" textRole="heading">
          {current ? `Edit ${current.label}` : "Add a tag"}
        </Text>
        <Text as="p" textRole="compact" tone="secondary">
          Slugs are what classifiers and saved clips use. Labels are what people see.
        </Text>
      </Stack>
      <div className="grid gap-3 sm:grid-cols-2">
        <form.Field name="label">
          {(field) => (
            <Field
              id="taxon-label"
              label="Label"
              onChangeText={(text) => edit(() => field.handleChange(text))}
              value={field.state.value}
            />
          )}
        </form.Field>
        <form.Field name="slug">
          {(field) => (
            <Field
              autoCapitalize="none"
              autoCorrect={false}
              disabled={editor.mode === "edit"}
              id="taxon-slug"
              label="Slug"
              onChangeText={(text) =>
                edit(() => field.handleChange(text.toLowerCase().replace(/[^a-z0-9_-]/g, "-")))
              }
              placeholder="breakfast-cereal"
              value={field.state.value}
            />
          )}
        </form.Field>
        <Stack gap="1">
          <BrowserSelectControl
            appearance="field"
            disabled={Boolean(current && nodeHasDescendants)}
            label="Axis"
            onValueChange={(value) => {
              edit(() => {
                form.setFieldValue("axis", value);
                form.setFieldValue("parent", "");
              });
            }}
            options={AXES.map((value) => ({ label: AXIS_COPY[value].label, value }))}
            value={axis}
          />
          {current && nodeHasDescendants ? (
            <Text as="p" textRole="hint" tone="secondary">
              Move this tag&rsquo;s children before changing its axis.
            </Text>
          ) : null}
        </Stack>
        <BrowserSelectControl
          appearance="field"
          label="Parent"
          onValueChange={(value) => edit(() => form.setFieldValue("parent", value))}
          options={[
            { label: "Top level", value: "" },
            ...parentOptions.map((taxon) => ({ label: `${taxon.label} (${taxon.slug})`, value: taxon.slug })),
          ]}
          value={parent}
        />
        <form.Field name="synonyms">
          {(field) => (
            <Field
              autoCapitalize="none"
              id="taxon-synonyms"
              label="Classifier synonyms"
              onChangeText={(text) => edit(() => field.handleChange(text))}
              placeholder="soda, soft drink"
              value={field.state.value}
            />
          )}
        </form.Field>
        <form.Field name="aliases">
          {(field) => (
            <Field
              autoCapitalize="none"
              autoCorrect={false}
              id="taxon-aliases"
              label="Retired slugs"
              onChangeText={(text) => edit(() => field.handleChange(text))}
              placeholder="old-machine-name"
              value={field.state.value}
            />
          )}
        </form.Field>
      </div>
      <Text as="p" textRole="hint" tone="secondary">
        Synonyms help the classifier resolve ordinary wording. Retired slugs preserve old integrations; both
        must be unique across the whole vocabulary.
      </Text>
      {review ? (
        <Card
          aria-label="Change impact"
          aria-live="polite"
          className="border-signal/40! bg-signal/5!"
          gap={12}
          padding={16}
        >
          <Stack gap="1">
            <Text as="h4" textRole="heading">
              Review the impact
            </Text>
            <Text as="p" textRole="compact" tone="secondary">
              {review.impact.affectedStoredClips > 0
                ? `${review.impact.affectedStoredClips.toLocaleString()} stored clips may have different inherited classification (${review.impact.directStoredClips.toLocaleString()} direct, ${review.impact.descendantStoredClips.toLocaleString()} through descendants).`
                : "No stored clip classifications will change."}
            </Text>
            <Text as="p" textRole="compact" tone="secondary">
              {review.impact.affectedPlayableClips > 0
                ? `${review.impact.affectedPlayableClips.toLocaleString()} playable clips will be checked against channel filler rules after this change.`
                : "No playable clips need channel eligibility checks."}
            </Text>
          </Stack>
          {review.impact.descendants.length > 0 ? (
            <Text as="p" textRole="compact">
              Descendants kept: {review.impact.descendants.map((node) => node.label).join(", ")}
            </Text>
          ) : null}
          {review.impact.savedChannelSelections.length > 0 ? (
            <Stack gap="1">
              <Text as="p" textRole="cardLabel">
                Saved channel selections that reference this branch
              </Text>
              <Text as="ul" className="list-disc pl-5" textRole="compact">
                {review.impact.savedChannelSelections.map((channel) => (
                  <li key={channel.id}>
                    Channel {channel.number}: {channel.name}
                  </li>
                ))}
              </Text>
            </Stack>
          ) : (
            <Text as="p" textRole="compact" tone="secondary">
              No saved channel selections reference this branch.
            </Text>
          )}
          {review.impact.resolverTermsAdded.length > 0 || review.impact.resolverTermsRemoved.length > 0 ? (
            <Stack gap="1">
              <Text as="p" textRole="cardLabel">
                Classifier wording
              </Text>
              {review.impact.resolverTermsAdded.length > 0 ? (
                <Text as="p" textRole="compact">
                  Starts resolving: {review.impact.resolverTermsAdded.join(", ")}
                </Text>
              ) : null}
              {review.impact.resolverTermsRemoved.length > 0 ? (
                <Text as="p" textRole="compact">
                  Stops resolving: {review.impact.resolverTermsRemoved.join(", ")}
                </Text>
              ) : null}
            </Stack>
          ) : (
            <Text as="p" textRole="compact" tone="secondary">
              Classifier wording is unchanged.
            </Text>
          )}
          {review.impact.deleteBlocked ? (
            <Text as="p" textRole="compact" tone="warning">
              Retag {review.impact.directStoredClips.toLocaleString()} directly assigned stored clips before
              removing this tag. This includes clips in Incoming, removed clips, and compilations.
            </Text>
          ) : null}
        </Card>
      ) : null}
      <Stack align="center" direction="row" gap="inline" wrap>
        {current ? (
          <Action
            disabled={busy}
            onPress={() => requestReview("delete", { operation: "delete", slug: current.slug })}
            variant="ghost"
          >
            Review removal
          </Action>
        ) : (
          <Stack marginRight="auto" render="span" />
        )}
        {review ? (
          <>
            <Action disabled={busy} onPress={() => setReview(undefined)} variant="outline">
              Back
            </Action>
            <Action
              disabled={busy || review.impact.deleteBlocked}
              onPress={applyReview}
              {...(review.action === "delete" ? { variant: "outline", tone: "danger" } : {})}
            >
              {busy
                ? "Applying…"
                : review.action === "delete"
                  ? "Confirm removal"
                  : editor.mode === "create"
                    ? "Confirm add"
                    : "Confirm update"}
            </Action>
          </>
        ) : (
          <>
            <Action disabled={busy} onPress={onClose} variant="outline">
              Cancel
            </Action>
            <Action
              disabled={busy || !label.trim() || (editor.mode === "create" && !slug.trim())}
              onPress={() => requestReview("save", saveCommand())}
            >
              {busy ? "Checking impact…" : "Review changes"}
            </Action>
          </>
        )}
      </Stack>
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
  if (!data) {
    return (
      <Text as="p" textRole="compact" tone="secondary">
        Loading the taxonomy…
      </Text>
    );
  }

  return (
    <Stack gap="section">
      <div className="grid gap-3 sm:grid-cols-3">
        <Card gap={4} padding={16}>
          <Text as="p" textRole="hint" tone="secondary">
            Library coverage
          </Text>
          {data.totalClips === 0 ? (
            <>
              <Text as="p" textRole="title">
                No playable clips yet
              </Text>
              <Text as="p" textRole="compact" tone="secondary">
                Coverage begins after clips are ready.
              </Text>
            </>
          ) : (
            <>
              <Text as="p" textRole="display">
                {data.taggedClips.toLocaleString()} / {data.totalClips.toLocaleString()}
              </Text>
              <Text as="p" textRole="compact" tone="secondary">
                playable clips have at least one classification signal
              </Text>
            </>
          )}
        </Card>
        <Card className="sm:col-span-2" gap={4} padding={16}>
          <Text as="p" textRole="heading">
            What belongs here
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            Classification describes what a clip contains. Kind is the closed playout role; format and
            presentation are optional browsing details. Era, audience, and grounded brand remain separate
            facts too.
          </Text>
          <Stack align="start" marginTop={4}>
            {data.totalClips === 0 ? (
              <Text as="p" textRole="compact" tone="secondary">
                Nothing to classify yet.
              </Text>
            ) : data.unclassifiedClips > 0 ? (
              <Link
                to="/filler/library"
                search={{ unclassified: true }}
                className="inline-flex underline-offset-2 hover:underline"
              >
                <Text as="span" textRole="compact" tone="signal">
                  Review {data.unclassifiedClips.toLocaleString()} unclassified{" "}
                  {data.unclassifiedClips === 1 ? "clip" : "clips"}
                </Text>
              </Link>
            ) : (
              <Text as="p" textRole="compact" tone="success">
                Every playable clip has at least one taxonomy tag.
              </Text>
            )}
          </Stack>
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
        <Stack gap="1">
          <Text as="h2" id="classification-coverage-heading" textRole="headline" strong>
            Classification coverage
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            These are independent signals. Missing presentation, seasonal, or audience cues can be perfectly
            normal.
          </Text>
        </Stack>
        {data.totalClips === 0 ? (
          <Card marginTop={12} padding={16}>
            <Text as="p" textRole="compact" tone="secondary">
              Coverage will appear after clips are ready. The vocabulary below is ready when they are.
            </Text>
          </Card>
        ) : (
          <div className="mt-3 grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
            {AXES.map((axis) => {
              const coverage = data.axisCoverage?.find((item) => item.axis === axis);
              return (
                <Card gap={4} key={axis} padding={16}>
                  <Text as="h3" textRole="heading">
                    {AXIS_COPY[axis].label}
                  </Text>
                  <Text as="p" textRole="compact" tone="secondary">
                    {AXIS_COPY[axis].help}
                  </Text>
                  <Stack marginTop={4}>
                    <Text as="p" textRole="hint" tone="secondary">
                      {AXIS_COPY[axis].example}
                    </Text>
                  </Stack>
                  {coverage ? (
                    <Stack
                      align="start"
                      borderColor="borderDecorative"
                      borderTopWidth={1}
                      gap="1"
                      marginTop={8}
                      paddingTop={12}
                    >
                      <Text as="p" textRole="compact">
                        <Text as="span" textRole="cardLabel">
                          {coverage.taggedClips.toLocaleString()}
                        </Text>{" "}
                        of {data.totalClips.toLocaleString()} playable clips
                      </Text>
                      {coverage.untaggedClips > 0 ? (
                        <Link
                          to="/filler/library"
                          search={{ withoutAxis: axis }}
                          className="inline-flex underline-offset-2 hover:underline"
                        >
                          <Text as="span" textRole="hint" tone="signal">
                            Browse {coverage.untaggedClips.toLocaleString()} without this signal
                          </Text>
                        </Link>
                      ) : (
                        <Text as="p" textRole="hint" tone="success">
                          All playable clips have this signal.
                        </Text>
                      )}
                    </Stack>
                  ) : null}
                </Card>
              );
            })}
          </div>
        )}
      </section>

      <details className="border-border bg-panel rounded-lg border">
        <Text
          as="summary"
          className="focus-visible:ring-ring cursor-pointer px-4 py-3 focus-visible:ring-2 focus-visible:outline-none"
          textRole="heading"
        >
          {isAdmin ? "Manage vocabulary" : "Browse vocabulary"}
        </Text>
        <Stack borderColor="borderDecorative" borderTopWidth={1} gap="4" padding={16}>
          <Text as="p" textRole="compact" tone="secondary">
            {isAdmin
              ? "Advanced: change the hierarchy and the words classifiers resolve. Review the impact before any edit is applied."
              : "This hierarchy is read-only. Administrators can change classifier vocabulary and relationships."}
          </Text>
          <div className="grid gap-4 xl:grid-cols-2">
            {AXES.map((axis) => {
              return (
                <Card key={axis} overflow="hidden">
                  <Stack
                    align="start"
                    borderBottomWidth={1}
                    borderColor="borderDecorative"
                    direction="row"
                    gap="control"
                    padding={16}
                  >
                    <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minWidth={0}>
                      <Text as="h3" textRole="heading">
                        {AXIS_COPY[axis].label}
                      </Text>
                      <Stack marginTop={2}>
                        <Text as="p" textRole="compact" tone="secondary">
                          {AXIS_COPY[axis].help}
                        </Text>
                      </Stack>
                      <Stack marginTop={4}>
                        <Text as="p" textRole="hint" tone="secondary">
                          {AXIS_COPY[axis].example}
                        </Text>
                      </Stack>
                    </Stack>
                    {isAdmin ? (
                      <Action
                        icon="plus"
                        onPress={() => setEditor({ mode: "create", axis })}
                        variant="outline"
                      >
                        Add
                      </Action>
                    ) : null}
                  </Stack>
                  {byAxis[axis].length === 0 ? (
                    <Stack paddingHorizontal={16} paddingVertical={12}>
                      <Text as="p" textRole="compact" tone="secondary">
                        No terms yet.
                      </Text>
                    </Stack>
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
        </Stack>
      </details>
    </Stack>
  );
};

export { TaxonomyTab };
