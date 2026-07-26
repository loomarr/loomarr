// The generated secrets (config-design §4). Their display policy differs by
// PURPOSE, which is why this is a closed set rather than a generic list.
type SecretName = "api_token" | "session_secret";

interface SecretRow {
  name: SecretName;
  label: string;
  // What this secret is for, in the operator's terms.
  purpose: string;
  // What regenerating it breaks, stated BEFORE the click (§4 side-effects).
  consequence: string;
  // SESSION_SECRET has nothing to paste anywhere, so it is never displayed (§4) —
  // Regenerate is its only affordance.
  displayable: boolean;
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
