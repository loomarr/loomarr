interface FinishOptions {
  to: "/suggest" | "/channels";
  // Prefills the Suggest workspace from a template (§13's blank-page killer).
  intent?: string;
}

export type { FinishOptions };
