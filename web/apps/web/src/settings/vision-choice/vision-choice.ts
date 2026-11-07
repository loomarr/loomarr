import { SettingKey } from "@loomarr/api/models/settingKey";

// The vision model as the two settings store it (filler.vision.provider, filler.vision.model).
// `inherit` with no model reuses the lineup model; `inherit` with a model is another model on
// the lineup's own provider; `ollama` is a local Ollama model whatever the lineup uses.
type VisionChoice = { provider: "inherit" | "ollama"; model: string };

const REUSE_LINEUP: VisionChoice = { provider: "inherit", model: "" };

// The stored pair as the picker compares it. Under a local lineup, the lineup's provider IS
// Ollama, so `inherit` with a model and `ollama` with that model are the same choice.
const visionChoiceOf = (provider: string, model: string, lineupHosted: boolean): VisionChoice => {
  if (provider !== "ollama" && model === "") return REUSE_LINEUP;
  if (provider === "ollama" || !lineupHosted) return { provider: "ollama", model };
  return { provider: "inherit", model };
};

const sameVisionChoice = (a: VisionChoice, b: VisionChoice) =>
  a.provider === b.provider && a.model === b.model;

// The setting writes that store a choice.
const visionChoiceSettings = (choice: VisionChoice): [SettingKey, string][] => [
  [SettingKey.FillerVisionProvider, choice.provider],
  [SettingKey.FillerVisionModel, choice.model],
];

export type { VisionChoice };
export { REUSE_LINEUP, sameVisionChoice, visionChoiceOf, visionChoiceSettings };
