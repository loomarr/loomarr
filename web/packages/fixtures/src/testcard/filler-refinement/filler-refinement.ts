import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import type { ListTaxonomyOutputBody } from "@loomarr/api/models/listTaxonomyOutputBody";
import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import type { SettingsListOutputBody } from "@loomarr/api/models/settingsListOutputBody";
import { taggedClip, untaggedClip } from "../testcard";

const fillerRefinementKnownClip: ClipDTO = {
  ...taggedClip,
  brand: "Sunny D",
  assertedTags: ["soft_drinks"],
  tags: ["soft_drinks", "food_drink"],
  source: "classic-ads",
  sourceUrl: "https://archive.org/details/classic-commercials",
  enrichment: {
    state: "complete",
    facts: [
      { axis: "kind", evidence: "item_metadata" },
      { axis: "era", evidence: "item_metadata" },
      { axis: "audience", evidence: "inference" },
      { axis: "brand", evidence: "content_observation" },
      { axis: "geography", evidence: "trusted_mapping" },
      { axis: "product", evidence: "item_metadata" },
    ],
  },
};
const fillerRefinementUnknownClip: ClipDTO = {
  ...untaggedClip,
  kind: "unclassified",
  source: "filler-dir",
  enrichment: { state: "details_limited" },
};

const fillerRefinementTaxonomy: ListTaxonomyOutputBody = {
  taxa: [
    {
      slug: "food_drink",
      label: "Food & drink",
      axis: "product",
      assertedClips: 0,
      matchedClips: 1,
      storedClips: 1,
    },
    {
      slug: "soft_drinks",
      label: "Soft drinks",
      axis: "product",
      parent: "food_drink",
      assertedClips: 1,
      matchedClips: 1,
      storedClips: 1,
    },
  ],
  totalClips: 1,
  taggedClips: 1,
  unclassifiedClips: 0,
  axisCoverage: [],
};

const fillerSetting = (
  key: string,
  label: string,
  value: string,
  kind: SettingEntry["kind"],
  advanced = false,
): SettingEntry => ({
  key,
  label,
  value,
  kind,
  advanced,
  group: "filler",
  owner:
    key.includes("max_catalog") || key.startsWith("filler.storage.")
      ? "filler.storage"
      : key.startsWith("filler.research.")
        ? "filler.details"
        : "filler.downloads",
  doc: "",
  provenance: "default",
  apply: "live",
  secret: false,
  set: true,
});
const fillerRefinementSettings: SettingsListOutputBody = {
  features: { filler: true },
  settings: [
    fillerSetting("filler.fetch.every", "Check frequency", "6h", "duration"),
    fillerSetting("filler.fetch.max_per_run", "New clips", "10", "int"),
    fillerSetting("filler.fetch.max_catalog_clips", "Catalog limit", "500", "int", true),
    fillerSetting("filler.storage.library_budget_gb", "Filler storage allowance", "0", "int"),
    {
      ...fillerSetting("filler.research.enabled", "Find missing clip details", "true", "bool"),
      presentation: "switch",
    },
    ...(
      [
        ["filler.research.wikidata_enabled", "Wikidata"],
        ["filler.research.wikipedia_enabled", "Wikipedia"],
        ["filler.research.archive_enabled", "Archive.org"],
        ["filler.research.loc_enabled", "Library of Congress"],
      ] as const
    ).map(([key, label]) => ({
      ...fillerSetting(key, label, "true", "bool", true),
      presentation: "switch" as const,
    })),
    fillerSetting("filler.research.web_provider", "Web search provider", "none", "enum", true),
    {
      ...fillerSetting("filler.research.brave_api_key", "Brave Search API key", "", "secret", true),
      secret: true,
      set: false,
    },
    fillerSetting("filler.research.searxng_url", "SearXNG address", "", "url", true),
    fillerSetting("filler.research.monthly_limit", "Monthly web searches", "100", "int", true),
  ],
};

export {
  fillerRefinementKnownClip,
  fillerRefinementSettings,
  fillerRefinementTaxonomy,
  fillerRefinementUnknownClip,
};
