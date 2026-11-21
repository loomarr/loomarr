import { ClipDTORole } from "@loomarr/api/models/clipDTORole";
import { ListFillerAudience } from "@loomarr/api/models/listFillerAudience";
import type { ListFillerParams } from "@loomarr/api/models/listFillerParams";
import type { ListFillerRole } from "@loomarr/api/models/listFillerRole";
import { isTaxonomyAxis, type TaxonomyAxis } from "../taxonomy-axes";

type FillerSearch = {
  q?: string;
  role?: string;
  audience?: string;
  taxon?: string;
  unclassified?: boolean;
  withoutAxis?: TaxonomyAxis;
  untagged?: boolean;
  view?: string;
  page?: number;
  parent?: string;
};

// The generated enums, so a new role (#2037) or audience is a valid link without a second list here.
const ROLES: string[] = Object.values(ClipDTORole);
const AUDIENCES: string[] = Object.values(ListFillerAudience);

const validateCatalogSearch = (search: Record<string, unknown>): FillerSearch => {
  const q = typeof search.q === "string" && search.q ? search.q : undefined;
  const role = ROLES.includes(search.role as string) ? (search.role as string) : undefined;
  const audience = AUDIENCES.includes(search.audience as string) ? (search.audience as string) : undefined;
  const taxon =
    typeof search.taxon === "string" && /^[a-z0-9_-]{1,64}$/.test(search.taxon) ? search.taxon : undefined;
  const unclassified = search.unclassified === true || search.unclassified === "true" ? true : undefined;
  const withoutAxis = isTaxonomyAxis(search.withoutAxis) ? search.withoutAxis : undefined;
  const untagged = search.untagged === true || search.untagged === "true" ? true : undefined;
  const view = search.view === "list" ? "list" : undefined;
  const rawPage = Number(search.page);
  const page = Number.isFinite(rawPage) && rawPage > 1 ? Math.floor(rawPage) : undefined;
  const parent = typeof search.parent === "string" && search.parent.length <= 128 ? search.parent : undefined;
  return {
    ...(q ? { q } : {}),
    ...(role ? { role } : {}),
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

// How many clips one page of the catalog holds (§10 V51d).
//
// ⚠ **ONE size for both the grid and the list**, deliberately. A per-view size would renumber the
// pages under the operator the moment they switched rendering — page 3 of the grid and page 3 of
// the list would be different clips — and `pageSize` is kept out of the URL for the same reason.
// 60 divides evenly by the grid's 2/3/4 columns, so the last row is never a ragged single card.
const CATALOG_PAGE_SIZE = 60;

// The one page of the catalog a search asks for. The catalog reads it and the Library route's
// loader warms it, so both name the same query key.
//
// ⚠ The catalog endpoint caps at 500 and defaults to 100 (§10 V51d), so an un-paged read would
// silently show the first hundred clips and call it the catalog.
const catalogListParams = (search: Partial<FillerSearch>): ListFillerParams => {
  const { q, role, audience, taxon, unclassified, withoutAxis, untagged, page = 1, parent } = search;
  const filtered = Boolean(q || role || audience || taxon || unclassified || withoutAxis || untagged);
  return {
    ...(q ? { q } : {}),
    ...(role ? { role: role as ListFillerRole } : {}),
    ...(audience ? { audience: audience as never } : {}),
    ...(taxon ? { taxon } : {}),
    ...(unclassified ? { unclassified: true } : {}),
    ...(withoutAxis ? { withoutAxis } : {}),
    ...(untagged ? { untagged: true } : {}),
    ...(parent ? { parentHash: parent } : !filtered ? { includeComposites: true, topLevel: true } : {}),
    limit: CATALOG_PAGE_SIZE,
    ...(page > 1 ? { offset: (page - 1) * CATALOG_PAGE_SIZE } : {}),
  };
};

export type { FillerSearch };
export { CATALOG_PAGE_SIZE, catalogListParams, validateCatalogSearch };
