type FillerSearch = {
  q?: string;
  kind?: string;
  audience?: string;
  taxon?: string;
  unclassified?: boolean;
  withoutAxis?: "product" | "format" | "seasonal" | "audience-cue";
  untagged?: boolean;
  view?: string;
  page?: number;
  parent?: string;
};

const KINDS = ["commercial", "bumper", "station_id", "psa", "trailer", "interstitial"];
const AUDIENCES = ["kids", "family", "general", "late_night"];
const TAXONOMY_AXES = ["product", "format", "seasonal", "audience-cue"] as const;

const validateCatalogSearch = (search: Record<string, unknown>): FillerSearch => {
  const q = typeof search.q === "string" && search.q ? search.q : undefined;
  const kind = KINDS.includes(search.kind as string) ? (search.kind as string) : undefined;
  const audience = AUDIENCES.includes(search.audience as string) ? (search.audience as string) : undefined;
  const taxon =
    typeof search.taxon === "string" && /^[a-z0-9_-]{1,64}$/.test(search.taxon) ? search.taxon : undefined;
  const unclassified = search.unclassified === true || search.unclassified === "true" ? true : undefined;
  const withoutAxis = TAXONOMY_AXES.includes(search.withoutAxis as (typeof TAXONOMY_AXES)[number])
    ? (search.withoutAxis as (typeof TAXONOMY_AXES)[number])
    : undefined;
  const untagged = search.untagged === true || search.untagged === "true" ? true : undefined;
  const view = search.view === "list" ? "list" : undefined;
  const rawPage = Number(search.page);
  const page = Number.isFinite(rawPage) && rawPage > 1 ? Math.floor(rawPage) : undefined;
  const parent = typeof search.parent === "string" && search.parent.length <= 128 ? search.parent : undefined;
  return {
    ...(q ? { q } : {}),
    ...(kind ? { kind } : {}),
    ...(audience ? { audience } : {}),
    ...(taxon ? { taxon } : {}),
    ...(unclassified ? { unclassified } : {}),
    ...(withoutAxis ? { withoutAxis } : {}),
    ...(untagged ? { untagged } : {}),
    ...(view ? { view } : {}),
    ...(page ? { page } : {}),
    ...(parent ? { parent } : {}),
  };
};

export type { FillerSearch };
export { validateCatalogSearch };
