interface FinishOptions {
  to: "/guide";
  // Stable identity for the wizard's preset handoff.
  preset?: string;
  // Retained for callers that hand off free text rather than a canonical preset.
  intent?: string;
}

export type { FinishOptions };
