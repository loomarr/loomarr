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
};
const fillerRefinementUnknownClip: ClipDTO = {
  ...untaggedClip,
  kind: "unclassified",
  source: "filler-dir",
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
  ],
};

export {
  fillerRefinementKnownClip,
  fillerRefinementSettings,
  fillerRefinementTaxonomy,
  fillerRefinementUnknownClip,
};
