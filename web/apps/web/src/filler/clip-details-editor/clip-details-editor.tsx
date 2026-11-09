import * as fillerApi from "@loomarr/api/endpoints/filler";
import { ClipDTOAudience } from "@loomarr/api/models/clipDTOAudience";
import { ClipDTOGeographicScope } from "@loomarr/api/models/clipDTOGeographicScope";
import { ClipDTORole } from "@loomarr/api/models/clipDTORole";
import type { PatchClipInputBodyRole } from "@loomarr/api/models/patchClipInputBodyRole";
import type { TaxonDTO } from "@loomarr/api/models/taxonDTO";
import { Action, Chip, DateField, Field } from "@loomarr/design-system";
import { useStore } from "@tanstack/react-form";
import { useEffect, useRef } from "react";
import { BrowserSelectControl } from "@/client-adapters";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { LocationPicker } from "@/components/loomarr/settings/installation-location";
import { useAppForm } from "@/lib/app-form";
import { TAXONOMY_AXES as AXES, type TaxonomyAxis } from "../taxonomy-axes";
import type { ClipDetailsEditorProps } from "./clip-details-editor.type";

// Unclassified shows but cannot be picked: it means detection has not named the role yet.
const ROLE_OPTIONS = [
  { disabled: true, label: "Not identified yet", value: ClipDTORole.unclassified },
  { label: "Commercial", value: ClipDTORole.commercial },
  { label: "Bumper", value: ClipDTORole.bumper },
  { label: "Station ID", value: ClipDTORole.station_id },
  { label: "PSA", value: ClipDTORole.psa },
  { label: "Trailer", value: ClipDTORole.trailer },
  { label: "Interstitial", value: ClipDTORole.interstitial },
  { label: "Political ad", value: ClipDTORole.political },
  { label: "News & weather", value: ClipDTORole.news },
];

const AUDIENCE_OPTIONS = [
  { label: "Not known", value: ClipDTOAudience[""] },
  { label: "Kids", value: ClipDTOAudience.kids },
  { label: "Family", value: ClipDTOAudience.family },
  { label: "General", value: ClipDTOAudience.general },
  { label: "Late night", value: ClipDTOAudience.late_night },
];

const SCOPE_OPTIONS = [
  { label: "Not known", value: ClipDTOGeographicScope.unknown },
  { label: "The whole country", value: ClipDTOGeographicScope.national },
  { label: "A local area", value: ClipDTOGeographicScope.local },
];

const AXIS_LABEL: Record<TaxonomyAxis, string> = {
  product: "Products & topics",
  format: "Format",
  presentation: "Presentation",
  seasonal: "Seasonal",
  "audience-cue": "Audience cue",
};

const flattenAxis = (taxa: TaxonDTO[], axis: string): Array<{ taxon: TaxonDTO; depth: number }> => {
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

// ClipDetailsEditor — hand-correct one clip's match tags (§10). Tags are what let the
// scheduler place a clip, so getting them right is the difference between a matched pod
// and the fallback ladder.
//
// `role` is editable here too: detection at sync mis-reads a trailer as a commercial
// often enough to matter, and the role decides where a clip sits in a pod — a bumper
// bookends a pod while a commercial fills it — so a wrong one yields structurally wrong pods.
const ClipDetailsEditor = ({ clip, onClose, onSaved }: ClipDetailsEditorProps) => {
  const editorRef = useRef<HTMLElement>(null);
  useEffect(() => {
    editorRef.current?.focus();
  }, []);
  const patch = fillerApi.useTagFillerClip({ mutation: { onSuccess: () => onSaved?.() } });
  const form = useAppForm({
    defaultValues: {
      name: clip?.name ?? "",
      role: (clip?.role ?? ClipDTORole.commercial) as ClipDTORole,
      era: clip?.era ? String(clip.era) : "",
      // ClipDTO's audience is optional AND includes "" for unset — "" is a real option here.
      audience: (clip?.audience ?? ClipDTOAudience[""]) as ClipDTOAudience,
      brand: clip?.brand ?? "",
      geoScope: (clip?.geographicScope ?? ClipDTOGeographicScope.unknown) as ClipDTOGeographicScope,
      country: clip?.country ?? "",
      network: clip?.network ?? "",
      // `station` on the clip is a directory id; the editor takes the call sign it held on the air date.
      station: clip?.stationCallSign ?? "",
      airDate: clip?.airDate ?? "",
      // Tags are the operator's chosen taxonomy leaves (§10 V45a) — a SET, not one category. Seeded
      // from the clip's asserted tags; `category` is derived server-side from these, never sent.
      // Never seed this editor from `tags`: that is the full match set, including inherited parents.
      // Writing it back would promote every derived rollup to a direct assertion, so later graph edits
      // could no longer distinguish what the operator chose from what the hierarchy implied.
      tags: clip?.assertedTags ?? ([] as string[]),
    },
    onSubmit: ({ value }) => {
      if (!clip) return;
      const { name, role, era, audience, brand, geoScope, country, network, station, airDate, tags } = value;
      patch.mutate({
        data: {
          // The clip is identified by `hash` in the body (§10 V45a) — no {id} URL segment.
          hash: clip.hash,
          // Sent only when changed: a sent name is recorded as curated (#1990), and saving another
          // detail must not vouch for a filename. An emptied name is sent so the server refuses it.
          ...(name.trim() === clip.name.trim() ? {} : { name: name.trim() }),
          // Unclassified means the optional role is unknown, not that this Ready clip needs approval.
          // Omit the role rather than asserting one the operator has not selected.
          ...(role === "unclassified" ? {} : { role: role as PatchClipInputBodyRole }),
          // An empty era means "unset", which the API takes as 0 — not "leave alone".
          era: era ? Number(era) : 0,
          audience,
          brand: brand.trim(),
          // The tag SET; the server grounds each and derives the category shadow.
          tags,
          geography: {
            scope: geoScope,
            country: geoScope === "unknown" ? undefined : country.trim().toUpperCase(),
            network: network.trim() || undefined,
            // An unchanged call sign sends the stored station id: a reused call sign with no air date
            // would otherwise resolve to two stations and refuse an unrelated edit.
            station:
              geoScope === "unknown"
                ? undefined
                : station.trim() === (clip.stationCallSign ?? "")
                  ? clip.station
                  : station.trim(),
            airDate: airDate || undefined,
          },
        },
      });
    },
  });
  const geoScope = useStore(form.store, (state) => state.values.geoScope);
  const tags = useStore(form.store, (state) => state.values.tags);
  const derivedTags = (clip?.tags ?? []).filter((tag) => !(clip?.assertedTags ?? []).includes(tag));

  // The tag vocabulary comes from the taxonomy graph — the ONE source of truth (§10 V45a), replacing
  // the old hardcoded CATEGORIES mirror. A member-readable read, so it loads for any operator.
  const vocab = fillerApi.useListTaxonomy();
  // The orval hook wraps the body: data.status===200 ? data.data.<body>. undefined while loading.
  const vocabTaxa = vocab.data?.status === 200 ? (vocab.data.data.taxa ?? []) : undefined;
  const derivedLabels = derivedTags.map(
    (slug) => vocabTaxa?.find((taxon) => taxon.slug === slug)?.label ?? slug,
  );
  if (!clip) return null;

  const toggleTag = (slug: string) =>
    form.setFieldValue("tags", (prev) =>
      prev.includes(slug) ? prev.filter((t) => t !== slug) : [...prev, slug],
    );

  return (
    // A labelled REGION, because the page already has a "Kind" and an "Audience" filter
    // with the same visible names. Without this scope, a screen-reader user hears two
    // identical controls and cannot tell which one edits the clip in front of them.
    <section
      ref={editorRef}
      tabIndex={-1}
      aria-label={`Edit details: ${clip.name}`}
      className="flex flex-col gap-4 outline-none"
    >
      <p className="text-muted-foreground text-sm">
        Change a detail if it is wrong. You do not need to fill in everything.
      </p>

      {patch.error != null && <ErrorState error={patch.error} />}

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <form.Field name="name">
          {(field) => (
            <Field
              description="The name viewers see during a break."
              id="tag-name"
              label="Name"
              maxLength={120}
              onChangeText={field.handleChange}
              placeholder="Clip name"
              value={field.state.value}
            />
          )}
        </form.Field>
        <form.Field name="role">
          {(field) => (
            <BrowserSelectControl
              appearance="field"
              label="Type"
              onValueChange={field.handleChange}
              options={ROLE_OPTIONS}
              value={field.state.value}
            />
          )}
        </form.Field>
        <form.Field name="era">
          {(field) => (
            <Field
              id="tag-era"
              keyboardType="number-pad"
              label="Year"
              maxLength={4}
              onChangeText={field.handleChange}
              placeholder="e.g. 1977"
              value={field.state.value}
            />
          )}
        </form.Field>
        <form.Field name="audience">
          {(field) => (
            <BrowserSelectControl
              appearance="field"
              label="Audience"
              onValueChange={field.handleChange}
              options={AUDIENCE_OPTIONS}
              value={field.state.value}
            />
          )}
        </form.Field>
        <form.Field name="brand">
          {(field) => (
            <Field
              description="The advertiser or sponsor shown in the clip."
              id="tag-brand"
              label="Advertiser"
              maxLength={120}
              onChangeText={field.handleChange}
              placeholder="Advertiser or sponsor"
              value={field.state.value}
            />
          )}
        </form.Field>
      </div>

      <details className="border-border rounded-lg border p-3">
        <summary className="cursor-pointer text-sm font-medium">Location and broadcast details</summary>
        <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <form.Field name="geoScope">
              {(field) => (
                <BrowserSelectControl
                  appearance="field"
                  label="Applies to"
                  onValueChange={field.handleChange}
                  options={SCOPE_OPTIONS}
                  value={field.state.value}
                />
              )}
            </form.Field>
          </div>
          <div className="sm:col-span-2">
            <form.Field name="country">
              {(field) => (
                <LocationPicker
                  value={{ country: geoScope === "unknown" ? "" : field.state.value }}
                  allowDetection={false}
                  emptyHint="Leave this unknown if you are not sure."
                  onChange={(location) => {
                    field.handleChange(location.country);
                    if (geoScope === "unknown") form.setFieldValue("geoScope", "national");
                  }}
                />
              )}
            </form.Field>
          </div>
          <form.Field name="network">
            {(field) => (
              <Field
                id="tag-network"
                label="Network (optional)"
                maxLength={120}
                onChangeText={field.handleChange}
                placeholder="Fox"
                value={field.state.value}
              />
            )}
          </form.Field>
          {/* A national clip keeps the station it was recorded off; a local one airs near it. */}
          {geoScope !== "unknown" && (
            <form.Field name="station">
              {(field) => (
                <Field
                  autoCapitalize="characters"
                  id="tag-station"
                  label={geoScope === "local" ? "Station call sign" : "Station call sign (optional)"}
                  maxLength={120}
                  onChangeText={field.handleChange}
                  placeholder="WEWS"
                  value={field.state.value}
                />
              )}
            </form.Field>
          )}
          <form.Field name="airDate">
            {(field) => (
              <DateField
                id="tag-air-date"
                label="Air date (optional)"
                onValueChange={field.handleChange}
                value={field.state.value}
              />
            )}
          </form.Field>
          <p className="text-muted-foreground text-xs sm:col-span-2">
            Use the area this clip was made for, not necessarily your home location. Leave it unknown if you
            are not sure.
          </p>
        </div>
      </details>

      {/* Tags: the taxonomy vocabulary as toggleable chips, grouped by axis (§10 V45a). This
            REPLACES the free-text category input — a tag must be a real taxon, so a checklist of the
            actual vocabulary is both easier and impossible to mis-type. The category shadow is derived
            server-side, so it is not edited here. */}
      <details className="border-border rounded-lg border p-3">
        <summary className="cursor-pointer text-sm font-medium">Topics and tags</summary>
        <div className="mt-3 flex flex-col gap-3">
          <p className="text-muted-foreground text-xs">Choose what this clip shows or advertises.</p>
          {vocab.error != null && <ErrorState error={vocab.error} />}
          {vocabTaxa == null ? (
            <p className="text-muted-foreground text-sm">Loading the tag vocabulary…</p>
          ) : (
            AXES.map((axis) => {
              const inAxis = flattenAxis(vocabTaxa, axis);
              if (inAxis.length === 0) return null;
              return (
                <div key={axis} className="flex flex-col gap-1.5">
                  <span className="text-muted-foreground text-xs">{AXIS_LABEL[axis]}</span>
                  <div className="flex flex-wrap gap-1.5">
                    {inAxis.map(({ taxon: t, depth }) => {
                      const on = tags.includes(t.slug);
                      return (
                        <Chip key={t.slug} onPress={() => toggleTag(t.slug)} selected={on} shape="pill">
                          {depth > 0 ? `${"↳ ".repeat(Math.min(depth, 2))}${t.label}` : t.label}
                        </Chip>
                      );
                    })}
                  </div>
                </div>
              );
            })
          )}
        </div>
      </details>

      {derivedTags.length > 0 ? (
        <section className="border-border bg-surface/40 rounded-md border p-3" aria-label="Derived matches">
          <p className="text-sm font-medium">Derived matches — read only</p>
          <p className="text-muted-foreground mt-1 text-xs">
            Inherited from the observed facts above and updated automatically when the hierarchy changes.
          </p>
          <p className="mt-2 text-sm break-words">{derivedLabels.join(", ")}</p>
        </section>
      ) : null}

      <div className="flex justify-end gap-2">
        <Action onPress={onClose} variant="outline">
          Cancel
        </Action>
        <Action disabled={patch.isPending} onPress={() => void form.handleSubmit()}>
          {patch.isPending ? "Saving…" : "Save details"}
        </Action>
      </div>
    </section>
  );
};

export { ClipDetailsEditor };
