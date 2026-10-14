// The generated credentials exposed to operators (config-design §4). This is a closed set
// so retired or internal-only secrets cannot accidentally acquire frontend affordances.
type SecretName = "api_token" | "playout_token";

interface SecretRow {
  name: SecretName;
  label: string;
  // What this secret is for, in the operator's terms.
  purpose: string;
  // What regenerating it breaks, stated BEFORE the click (§4 side-effects).
  consequence: string;
}

interface SecretsPanelProps {
  secrets: SecretRow[];
  // Revealed values, keyed by name. Absent = not revealed yet.
  revealed: Partial<Record<SecretName, string>>;
  onReveal: (name: SecretName) => void;
  onRegenerate: (name: SecretName) => void;
  busy?: SecretName;
  className?: string;
}

export type { SecretName, SecretRow, SecretsPanelProps };
