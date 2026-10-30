import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import type { ClipGeographyDTO } from "@loomarr/api/models/clipGeographyDTO";
import type { PatchClipInputBodyKind } from "@loomarr/api/models/patchClipInputBodyKind";
import type { TaxonDTO } from "@loomarr/api/models/taxonDTO";
import { useEffect, useRef, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { LocationPicker } from "@/components/loomarr/settings/installation-location";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { TAXONOMY_AXES as AXES, type TaxonomyAxis } from "../taxonomy-axes";
import type { ClipDetailsEditorProps } from "./clip-details-editor.type";

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
// `kind` is editable here too: detection at sync mis-reads a trailer as a commercial
// often enough to matter, and kind drives pod ROLE — a bumper bookends a pod while a
// commercial fills it — so a wrong kind yields structurally wrong pods.
const ClipDetailsEditor = ({ clip, onClose, onSaved }: ClipDetailsEditorProps) => {
  const editorRef = useRef<HTMLElement>(null);
  useEffect(() => {
    editorRef.current?.focus();
  }, []);
  const [name, setName] = useState(clip?.name ?? "");
  const [kind, setKind] = useState(clip?.kind ?? "commercial");
  const [era, setEra] = useState(clip?.era ? String(clip.era) : "");
  // ClipDTO's audience is optional AND includes "" for unset, so the state is widened to
  // the select's own domain rather than the DTO's — "" is a real option here.
  const [audience, setAudience] = useState<string>(clip?.audience ?? "");
  const [brand, setBrand] = useState(clip?.brand ?? "");
  const [geoScope, setGeoScope] = useState<ClipGeographyDTO["scope"]>(clip?.geographicScope ?? "unknown");
  const [country, setCountry] = useState(clip?.country ?? "");
  const [network, setNetwork] = useState(clip?.network ?? "");
  // `station` on the clip is a directory id; the editor takes the call sign it held on the air date.
  const [station, setStation] = useState(clip?.stationCallSign ?? "");
  const [airDate, setAirDate] = useState(clip?.airDate ?? "");
  // Tags are the operator's chosen taxonomy leaves (§10 V45a) — a SET, not one category. Seeded from
  // the clip's asserted tags; `category` is derived server-side from these, never sent.
  // Never seed this editor from `tags`: that is the full match set, including inherited parents.
  // Writing it back would promote every derived rollup to a direct assertion, so later graph edits
  // could no longer distinguish what the operator chose from what the hierarchy implied.
  const [tags, setTags] = useState<string[]>(clip?.assertedTags ?? []);
  const derivedTags = (clip?.tags ?? []).filter((tag) => !(clip?.assertedTags ?? []).includes(tag));

  // The tag vocabulary comes from the taxonomy graph — the ONE source of truth (§10 V45a), replacing
  // the old hardcoded CATEGORIES mirror. A member-readable read, so it loads for any operator.
  const vocab = fillerApi.useListTaxonomy();
  // The orval hook wraps the body: data.status===200 ? data.data.<body>. undefined while loading.
  const vocabTaxa = vocab.data?.status === 200 ? (vocab.data.data.taxa ?? []) : undefined;
  const derivedLabels = derivedTags.map(
    (slug) => vocabTaxa?.find((taxon) => taxon.slug === slug)?.label ?? slug,
  );
  const patch = fillerApi.useTagFillerClip({ mutation: { onSuccess: () => onSaved?.() } });

  if (!clip) return null;

  const save = () => {
    patch.mutate({
      data: {
        // The clip is identified by `hash` in the body (§10 V45a) — no {id} URL segment.
        hash: clip.hash,
        // Sent only when changed: a sent name is recorded as curated (#1990), and saving another
        // detail must not vouch for a filename. An emptied name is sent so the server refuses it.
        ...(name.trim() === clip.name.trim() ? {} : { name: name.trim() }),
        // Unclassified means the optional role is unknown, not that this Ready clip needs approval.
        // Omit kind rather than asserting a role the operator has not selected.
        ...(kind === "unclassified" ? {} : { kind: kind as PatchClipInputBodyKind }),
        // An empty era means "unset", which the API takes as 0 — not "leave alone".
        era: era ? Number(era) : 0,
        audience: audience as ClipDTO["audience"],
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
  };

  const toggleTag = (slug: string) =>
    setTags((prev) => (prev.includes(slug) ? prev.filter((t) => t !== slug) : [...prev, slug]));

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
        <div>
          <Label htmlFor="tag-name">Name</Label>
          <Input
            id="tag-name"
            maxLength={120}
            placeholder="Clip name"
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
          <p className="text-muted-foreground mt-1 text-xs">The name viewers see during a break.</p>
        </div>
        <div>
          <Label htmlFor="tag-kind">Type</Label>
          <Select value={kind} onValueChange={(v) => setKind(v as ClipDTO["kind"])}>
            <SelectTrigger id="tag-kind">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="unclassified" disabled>
                Not identified yet
              </SelectItem>
              <SelectItem value="commercial">Commercial</SelectItem>
              <SelectItem value="bumper">Bumper</SelectItem>
              <SelectItem value="station_id">Station ID</SelectItem>
              <SelectItem value="psa">PSA</SelectItem>
              <SelectItem value="trailer">Trailer</SelectItem>
              <SelectItem value="interstitial">Interstitial</SelectItem>
              <SelectItem value="political">Political ad</SelectItem>
              <SelectItem value="news">News &amp; weather</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div>
          <Label htmlFor="tag-era">Year</Label>
          <Input
            id="tag-era"
            type="number"
            placeholder="e.g. 1977"
            value={era}
            onChange={(e) => setEra(e.target.value)}
          />
        </div>
        <div>
          <Label htmlFor="tag-audience">Audience</Label>
          {/* Radix reserves "" for clearing, so an "unset" sentinel stands in for the
                empty audience and maps back to "" in state. */}
          <Select value={audience || "unset"} onValueChange={(v) => setAudience(v === "unset" ? "" : v)}>
            <SelectTrigger id="tag-audience">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="unset">Not known</SelectItem>
              <SelectItem value="kids">Kids</SelectItem>
              <SelectItem value="family">Family</SelectItem>
              <SelectItem value="general">General</SelectItem>
              <SelectItem value="late_night">Late night</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div>
          <Label htmlFor="tag-brand">Advertiser</Label>
          <Input
            id="tag-brand"
            maxLength={120}
            placeholder="Advertiser or sponsor"
            value={brand}
            onChange={(event) => setBrand(event.target.value)}
          />
          <p className="text-muted-foreground mt-1 text-xs">The advertiser or sponsor shown in the clip.</p>
        </div>
      </div>

      <details className="border-border rounded-lg border p-3">
        <summary className="cursor-pointer text-sm font-medium">Location and broadcast details</summary>
        <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <Label htmlFor="tag-geographic-scope">Applies to</Label>
            <Select
              value={geoScope}
              onValueChange={(value) => setGeoScope(value as ClipGeographyDTO["scope"])}
            >
              <SelectTrigger id="tag-geographic-scope">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="unknown">Not known</SelectItem>
                <SelectItem value="national">The whole country</SelectItem>
                <SelectItem value="local">A local area</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="sm:col-span-2">
            <LocationPicker
              value={{ country: geoScope === "unknown" ? "" : country }}
              allowDetection={false}
              emptyHint="Leave this unknown if you are not sure."
              onChange={(location) => {
                setCountry(location.country);
                if (geoScope === "unknown") setGeoScope("national");
              }}
            />
          </div>
          <div>
            <Label htmlFor="tag-network">Network (optional)</Label>
            <Input
              id="tag-network"
              maxLength={120}
              value={network}
              onChange={(event) => setNetwork(event.target.value)}
              placeholder="Fox"
            />
          </div>
          {geoScope !== "unknown" && (
            <div>
              {/* A national clip keeps the station it was recorded off; a local one airs near it. */}
              <Label htmlFor="tag-station">
                {geoScope === "local" ? "Station call sign" : "Station call sign (optional)"}
              </Label>
              <Input
                id="tag-station"
                maxLength={120}
                value={station}
                onChange={(event) => setStation(event.target.value)}
                placeholder="WEWS"
              />
            </div>
          )}
          <div>
            <Label htmlFor="tag-air-date">Air date (optional)</Label>
            <Input
              id="tag-air-date"
              type="date"
              value={airDate}
              onChange={(event) => setAirDate(event.target.value)}
            />
          </div>
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
                  <span className="text-muted-foreground text-xs tracking-wide uppercase">
                    {AXIS_LABEL[axis]}
                  </span>
                  <div className="flex flex-wrap gap-1.5">
                    {inAxis.map(({ taxon: t, depth }) => {
                      const on = tags.includes(t.slug);
                      return (
                        <button
                          key={t.slug}
                          type="button"
                          aria-pressed={on}
                          title={t.parent ? `${t.label}, under ${t.parent}` : `${t.label}, top level`}
                          onClick={() => toggleTag(t.slug)}
                          className={
                            // Pressed is neutral (#1909 Q2): amber is focus or the primary action.
                            on
                              ? "border-foreground bg-static-800 text-foreground rounded-full border px-2.5 py-0.5 text-xs font-semibold"
                              : "border-border text-muted-foreground hover:border-static-500 rounded-full border px-2.5 py-0.5 text-xs"
                          }
                        >
                          {depth > 0 ? `${"↳ ".repeat(Math.min(depth, 2))}${t.label}` : t.label}
                        </button>
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
        <Button variant="outline" size="sm" onClick={onClose}>
          Cancel
        </Button>
        <Button size="sm" disabled={patch.isPending} onClick={save}>
          {patch.isPending ? "Saving…" : "Save details"}
        </Button>
      </div>
    </section>
  );
};

export { ClipDetailsEditor };
