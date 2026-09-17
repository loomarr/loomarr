const TECHNICAL_EXPLANATION =
  /\b(?:catalog|constituent|evidence|grounded|grounding|public[- ]reference|resolved|source truth)\b/i;
const MACHINE_TITLE = /^(?:movie|series):(?:tmdb|tvdb):\d+$/i;

const friendlyTitleRationale = (value: string) => {
  const knownExplanations: Record<string, string> = {
    "Included because your curated-title subject resolves to this Catalog title.":
      "This is the title you asked for.",
    "Included because the resolved public reference names this title as a constituent.":
      "This title is part of the lineup or collection you asked for.",
    "Included because you supplied this title as a constituent of the named set.":
      "You named this title as part of the lineup or collection.",
  };
  if (knownExplanations[value]) return knownExplanations[value];
  if (TECHNICAL_EXPLANATION.test(value)) return "This title matches the channel you described.";
  return value;
};

const friendlyProposalRationale = (value: string) => {
  const knownExplanations: Record<string, string> = {
    "Every offered title is backed by user-supplied or resolved public-reference constituent evidence.":
      "These titles come from the lineup or collection you asked for and the examples you provided.",
    "Every offered title is backed by resolved public-reference constituent evidence.":
      "These titles are part of the lineup or collection you asked for.",
    "Every offered title is backed by user-supplied constituent evidence.":
      "These titles are based on the examples you provided.",
  };
  if (knownExplanations[value]) return knownExplanations[value];
  if (TECHNICAL_EXPLANATION.test(value)) return "These titles match the channel you described.";
  return value;
};

const friendlyCandidateName = (value?: string) =>
  !value || MACHINE_TITLE.test(value) ? "Unidentified title" : value;

export { friendlyCandidateName, friendlyProposalRationale, friendlyTitleRationale };
