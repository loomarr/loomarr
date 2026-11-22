import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { TaxonDTO } from "@loomarr/api/models/taxonDTO";
import type { TaxonomyImpactCommandDTO } from "@loomarr/api/models/taxonomyImpactCommandDTO";
import { toProblem } from "@loomarr/api/mutator";
import { Action, Card, Field, Grid, Stack, Text } from "@loomarr/design-system";
import { useStore } from "@tanstack/react-form";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { BrowserSelectControl } from "@/client-adapters";
import { useAppForm } from "@/lib/app-form";
import { TAXONOMY_AXES as AXES, type TaxonomyAxis as Axis } from "../../taxonomy-axes";
import { AXIS_COPY } from "../taxonomy-axis-copy";
import type { Editor, Review } from "./taxon-editor.type";
import { TaxonImpactReview } from "./taxon-impact-review";

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
      <Grid columns={{ base: 1, sm: 2 }} gap="3">
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
      </Grid>
      <Text as="p" textRole="hint" tone="secondary">
        Synonyms help the classifier resolve ordinary wording. Retired slugs preserve old integrations; both
        must be unique across the whole vocabulary.
      </Text>
      {review ? <TaxonImpactReview impact={review.impact} /> : null}
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

export { TaxonEditor };
