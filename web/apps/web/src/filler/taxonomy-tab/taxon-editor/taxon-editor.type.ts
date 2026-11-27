import type { TaxonDTO } from "@loomarr/api/models/taxonDTO";
import type { TaxonomyImpactCommandDTO } from "@loomarr/api/models/taxonomyImpactCommandDTO";
import type { TaxonomyImpactDTO } from "@loomarr/api/models/taxonomyImpactDTO";
import type { TaxonomyAxis as Axis } from "../../taxonomy-axes";

type Editor = { mode: "create" | "edit"; axis: Axis; taxon?: TaxonDTO };
type Review = { action: "save" | "delete"; command: TaxonomyImpactCommandDTO; impact: TaxonomyImpactDTO };

export type { Editor, Review };
