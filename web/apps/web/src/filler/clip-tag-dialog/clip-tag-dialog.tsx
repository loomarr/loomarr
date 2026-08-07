import { type ClipDTO, fillerApi } from "@loomarr/api";
import { useState } from "react";
import { ErrorState } from "@/components/loomarr";
import {
  Button,
  Card,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui";
import type { ClipTagDialogProps } from "./clip-tag-dialog.type";

// The taxonomy axes, in display order — the independent dimensions a clip is tagged on (§10 V45a).
// Ordered product-first because it is the deepest and most-used; the labels are the human forms.
const AXES = ["product", "format", "seasonal", "audience-cue"] as const;
const AXIS_LABEL: Record<(typeof AXES)[number], string> = {
  product: "Product",
  format: "Format",
  seasonal: "Seasonal",
  "audience-cue": "Audience cue",
};

// ClipTagDialog — hand-correct one clip's match tags (§10). Tags are what let the
// scheduler place a clip, so getting them right is the difference between a matched pod
// and the fallback ladder.
//
// `kind` is editable here too: detection at sync mis-reads a trailer as a commercial
// often enough to matter, and kind drives pod ROLE — a bumper bookends a pod while a
// commercial fills it — so a wrong kind yields structurally wrong pods.
const ClipTagDialog = ({ clip, onClose, onSaved }: ClipTagDialogProps) => {
  const [kind, setKind] = useState(clip?.kind ?? "commercial");
  const [era, setEra] = useState(clip?.era ? String(clip.era) : "");
  // ClipDTO's audience is optional AND includes "" for unset, so the state is widened to
  // the select's own domain rather than the DTO's — "" is a real option here.
  const [audience, setAudience] = useState<string>(clip?.audience ?? "");
  // Tags are the operator's chosen taxonomy leaves (§10 V45a) — a SET, not one category. Seeded from
  // the clip's asserted tags; `category` is derived server-side from these, never sent.
  const [tags, setTags] = useState<string[]>(clip?.tags ?? []);

  // The tag vocabulary comes from the taxonomy graph — the ONE source of truth (§10 V45a), replacing
  // the old hardcoded CATEGORIES mirror. A member-readable read, so it loads for any operator.
  const vocab = fillerApi.useListTaxonomy();
  // The orval hook wraps the body: data.status===200 ? data.data.<body>. undefined while loading.
  const vocabTaxa = vocab.data?.status === 200 ? (vocab.data.data.taxa ?? []) : undefined;
  const patch = fillerApi.useTagFillerClip({ mutation: { onSuccess: () => onSaved?.() } });

  if (!clip) return null;

  const save = () => {
    patch.mutate({
      data: {
        // The clip is identified by `hash` in the body (§10 V45a) — no {id} URL segment.
        hash: clip.hash,
        kind: kind as ClipDTO["kind"],
        // An empty era means "unset", which the API takes as 0 — not "leave alone".
        era: era ? Number(era) : 0,
        audience: audience as ClipDTO["audience"],
        // The tag SET; the server grounds each and derives the category shadow.
        tags,
      },
    });
  };

  const toggleTag = (slug: string) =>
    setTags((prev) => (prev.includes(slug) ? prev.filter((t) => t !== slug) : [...prev, slug]));

  return (
    // A labelled REGION, because the page already has a "Kind" and an "Audience" filter
    // with the same visible names. Without this scope, a screen-reader user hears two
    // identical controls and cannot tell which one edits the clip in front of them.
    <Card>
      <section aria-label={`Edit tags: ${clip.name}`} className="flex flex-col gap-4 p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="truncate font-semibold text-lg">{clip.name}</h2>
            {clip.aiTagged && (
              <p className="mt-1 text-muted-foreground text-sm">
                These tags were guessed by the AI. Saving confirms them as yours.
              </p>
            )}
          </div>
          <Button variant="ghost" size="sm" onClick={onClose}>
            Close
          </Button>
        </div>

        {patch.error != null && <ErrorState error={patch.error} />}

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <Label htmlFor="tag-kind">Kind</Label>
            <Select value={kind} onValueChange={(v) => setKind(v as ClipDTO["kind"])}>
              <SelectTrigger id="tag-kind">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="commercial">Commercial</SelectItem>
                <SelectItem value="bumper">Bumper</SelectItem>
                <SelectItem value="station_id">Station ID</SelectItem>
                <SelectItem value="psa">PSA</SelectItem>
                <SelectItem value="trailer">Trailer</SelectItem>
                <SelectItem value="interstitial">Interstitial</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div>
            <Label htmlFor="tag-era">Era</Label>
            <Input
              id="tag-era"
              type="number"
              placeholder="1994"
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
                <SelectItem value="unset">Unset</SelectItem>
                <SelectItem value="kids">Kids</SelectItem>
                <SelectItem value="family">Family</SelectItem>
                <SelectItem value="general">General</SelectItem>
                <SelectItem value="late_night">Late night</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        {/* Tags: the taxonomy vocabulary as toggleable chips, grouped by axis (§10 V45a). This
            REPLACES the free-text category input — a tag must be a real taxon, so a checklist of the
            actual vocabulary is both easier and impossible to mis-type. The category shadow is derived
            server-side, so it is not edited here. */}
        <fieldset className="flex flex-col gap-3">
          <legend className="font-medium text-sm">Tags</legend>
          {vocab.error != null && <ErrorState error={vocab.error} />}
          {vocabTaxa == null ? (
            <p className="text-muted-foreground text-sm">Loading the tag vocabulary…</p>
          ) : (
            AXES.map((axis) => {
              const inAxis = vocabTaxa.filter((t) => t.axis === axis);
              if (inAxis.length === 0) return null;
              return (
                <div key={axis} className="flex flex-col gap-1.5">
                  <span className="text-muted-foreground text-xs uppercase tracking-wide">
                    {AXIS_LABEL[axis]}
                  </span>
                  <div className="flex flex-wrap gap-1.5">
                    {inAxis.map((t) => {
                      const on = tags.includes(t.slug);
                      return (
                        <button
                          key={t.slug}
                          type="button"
                          aria-pressed={on}
                          onClick={() => toggleTag(t.slug)}
                          className={
                            on
                              ? "rounded-full border border-primary bg-primary/15 px-2.5 py-0.5 text-primary text-xs"
                              : "rounded-full border border-border px-2.5 py-0.5 text-muted-foreground text-xs hover:border-primary/50"
                          }
                        >
                          {t.label}
                        </button>
                      );
                    })}
                  </div>
                </div>
              );
            })
          )}
        </fieldset>

        <div className="flex justify-end gap-2">
          <Button variant="outline" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button size="sm" disabled={patch.isPending} onClick={save}>
            {patch.isPending ? "Saving…" : "Save tags"}
          </Button>
        </div>
      </section>
    </Card>
  );
};

export { ClipTagDialog };
