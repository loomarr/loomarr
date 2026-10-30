import type { TaxonDTOAxis } from "@loomarr/api/models/taxonDTOAxis";

type TaxonomyAxis = TaxonDTOAxis;

// The taxonomy axes, in display order — the independent dimensions a clip is tagged on (§10 V45a).
// Ordered product-first because it is the deepest and most-used. A Record over the generated enum,
// so a new backend axis fails the build here rather than silently missing from every editor.
const DISPLAY_ORDER: Record<TaxonomyAxis, true> = {
  product: true,
  format: true,
  presentation: true,
  seasonal: true,
  "audience-cue": true,
};

const TAXONOMY_AXES = Object.keys(DISPLAY_ORDER) as TaxonomyAxis[];

const isTaxonomyAxis = (value: unknown): value is TaxonomyAxis =>
  typeof value === "string" && Object.hasOwn(DISPLAY_ORDER, value);

export type { TaxonomyAxis };
export { isTaxonomyAxis, TAXONOMY_AXES };
