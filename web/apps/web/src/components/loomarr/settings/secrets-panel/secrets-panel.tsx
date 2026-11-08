import { Action } from "@loomarr/design-system";
import { useState } from "react";
import { useCopied } from "@/lib/use-copied";
import { cn } from "@/lib/utils";
import type { SecretName, SecretsPanelProps } from "./secrets-panel.type";

// Generated credentials (config-design §4), on Sonarr's model: values an operator must
// paste elsewhere are viewable on demand.
//
// Regeneration states its consequence BEFORE the click and requires a second confirm.
// That is not ceremony: rotating API_TOKEN or PLAYOUT_TOKEN breaks every consumer using
// it. An operator should learn that from the button, not from the aftermath.
const SecretsPanel = ({ secrets, revealed, onReveal, onRegenerate, busy, className }: SecretsPanelProps) => {
  const [confirming, setConfirming] = useState<SecretName | undefined>();
  const { copied, copy } = useCopied<SecretName>();

  return (
    <ul className={cn("flex flex-col gap-3", className)}>
      {secrets.map((secret) => {
        const value = revealed[secret.name];
        const isConfirming = confirming === secret.name;

        return (
          <li key={secret.name} className="border-border bg-card flex flex-col gap-2 rounded-md border p-3">
            <div>
              <p className="text-sm font-medium">{secret.label}</p>
              <p className="text-muted-foreground text-sm">{secret.purpose}</p>
            </div>

            <div className="flex items-center gap-2">
              <code className="border-input bg-static-800 flex-1 truncate rounded-md border px-3 py-1.5 font-mono text-sm">
                {value ?? "••••••••••••••••"}
              </code>
              <Action
                disabled={busy === secret.name}
                icon={value ? "conceal" : "reveal"}
                onPress={() => onReveal(secret.name)}
                variant="outline"
              >
                {value ? "Hide" : "Reveal"}
              </Action>
              {value && (
                <Action icon="copy" onPress={() => copy(value, secret.name)} variant="outline">
                  {copied === secret.name ? "Copied" : "Copy"}
                </Action>
              )}
            </div>

            {isConfirming ? (
              <div className="border-onair-tint-30 bg-onair-tint-8 flex flex-col gap-2 rounded-md border p-3">
                <p className="text-onair-300 text-sm">{secret.consequence}</p>
                <div className="flex items-center gap-2">
                  <Action
                    disabled={busy === secret.name}
                    onPress={() => {
                      onRegenerate(secret.name);
                      setConfirming(undefined);
                    }}
                    variant="destructive"
                  >
                    Regenerate anyway
                  </Action>
                  <Action onPress={() => setConfirming(undefined)} variant="ghost">
                    Cancel
                  </Action>
                </div>
              </div>
            ) : (
              <Action
                icon="refresh"
                onPress={() => setConfirming(secret.name)}
                style={{ alignSelf: "flex-start" }}
                variant="outline"
              >
                Regenerate
              </Action>
            )}
          </li>
        );
      })}
    </ul>
  );
};

export { SecretsPanel };
