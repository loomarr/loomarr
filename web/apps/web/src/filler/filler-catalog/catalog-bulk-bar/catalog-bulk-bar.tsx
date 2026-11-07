import * as fillerApi from "@loomarr/api/endpoints/filler";
import { ListFillerAudience } from "@loomarr/api/models/listFillerAudience";
import { toProblem } from "@loomarr/api/mutator";
import { isOk } from "@loomarr/api/unwrap";
import { pluralize } from "@loomarr/core/format";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useFillerInvalidate } from "../../use-filler-invalidate";

// The bulk bar's dropdowns (V35). ⚠ Each is INDEPENDENT — picking one sends only that field, and the
// server leaves the other alone. A single "apply" that posted both would blank whatever the operator
// had not touched, which is the failure the BE's per-field optionality exists to prevent.
//
// Bulk options omit unknown/unset values: a bulk correction must not accidentally blank optional
// metadata across a hundred clips. Unknown values remain quiet on ordinary cards.
// ⚠ Labelled "Set …", not "Era"/"Audience"/"Category". The catalog's FILTER bar already owns those
// names on this page, and two controls sharing an accessible name is a real ambiguity for anyone
// driving by keyboard or screen reader. The verb also says what each one does: the filter narrows
// what you see, this changes what the clips ARE.
// ⚠ Bulk "Set category" was REMOVED (§10 V45a). Category is a DERIVED shadow of the taxonomy tags —
// not a directly-settable field — and a single-value bulk menu cannot express a tag SET without either
// wiping a clip's other tags or inventing a questionable "add one tag to N clips" affordance. Tag
// editing is per-clip in the details editor, which serves the real vocabulary. Bulk era/audience stay:
// those ARE single closed-enum values a menu fits.
const BULK_TAG_FIELDS = [
  { key: "era", label: "Set era", options: ["1950", "1960", "1970", "1980", "1990", "2000", "2010", "2020"] },
  { key: "audience", label: "Set audience", options: Object.values(ListFillerAudience) },
] as const;

interface CatalogBulkBarProps {
  /** Content hashes of the selected clips (§10 V45a: the bulk endpoints key on hashes). */
  selected: ReadonlySet<string>;
  onClear: () => void;
}

// Shown only while something is selected, ABOVE the grid rather than floating over it: a bar that
// covers the cards hides the very thing the operator is deciding about. Owns the two bulk actions.
const CatalogBulkBar = ({ selected, onClear }: CatalogBulkBarProps) => {
  const { invalidateLifecycle } = useFillerInvalidate();

  // Bulk retag. Each field is independent on the server, so sending only what the operator
  // picked leaves the other alone rather than blanking it.
  const bulkTag = fillerApi.useBulkTagFiller({
    mutation: {
      onSuccess: (res) => {
        onClear();
        invalidateLifecycle();
        const updated = isOk(res) ? res.data.updated : 0;
        toast.success(`Retagged ${pluralize(updated, "clip")}`);
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't retag those clips"),
    },
  });

  // Removing a clip from the catalog (V35). ⚠ A TOMBSTONE on the server: the clip leaves the
  // catalog and stops being used in breaks, and the file is untouched. The copy here says
  // "catalog" for that reason and must keep saying it.
  const removeClips = fillerApi.useBulkRemoveFiller({
    mutation: {
      onSuccess: invalidateLifecycle,
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't remove those clips"),
    },
  });

  return (
    <div className="border-signal/40 bg-signal/5 flex flex-wrap items-center gap-3 rounded-lg border p-3">
      <span className="text-signal font-mono text-xs">{pluralize(selected.size, "clip")} selected</span>

      {BULK_TAG_FIELDS.map((field) => (
        <Select
          key={field.key}
          value=""
          onValueChange={(value) =>
            bulkTag.mutate({
              data: {
                hashes: [...selected],
                [field.key]: field.key === "era" ? Number(value) : value,
              },
            })
          }
        >
          <SelectTrigger className="w-auto min-w-32" aria-label={field.label}>
            <SelectValue placeholder={field.label} />
          </SelectTrigger>
          <SelectContent>
            {field.options.map((option) => (
              <SelectItem key={option} value={option}>
                {option}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ))}

      {/* ⚠ "Remove from catalog", not "Delete". The server's action is a TOMBSTONE: the
          clip stops appearing here and stops being used in breaks, and the file stays
          exactly where the operator put it. The label has to keep saying catalog. */}
      <Button
        variant="outline"
        size="sm"
        disabled={removeClips.isPending}
        onClick={() => removeClips.mutate({ data: { hashes: [...selected] } })}
        title="Stop using these clips. The files stay in your folder."
      >
        Remove from catalog
      </Button>

      <Button variant="ghost" size="sm" className="ml-auto" onClick={onClear}>
        Clear
      </Button>
    </div>
  );
};

export { BULK_TAG_FIELDS, CatalogBulkBar };
