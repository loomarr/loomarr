import { Action, Stack } from "@loomarr/design-system";
import { ConfirmStep } from "@loomarr/ui";
import { useState } from "react";
import { useCopied } from "@/lib/use-copied";
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
    <Stack className={className} gap="control" render="ul">
      {secrets.map((secret) => {
        const value = revealed[secret.name];
        const isConfirming = confirming === secret.name;

        return (
          <Stack
            className="border-border bg-card rounded-md border p-3"
            gap="inline"
            key={secret.name}
            render="li"
          >
            <div>
              <p className="text-sm font-medium">{secret.label}</p>
              <p className="text-muted-foreground text-sm">{secret.purpose}</p>
            </div>

            <Stack align="center" direction="row" gap="inline">
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
                <Action icon="copy" onPress={() => void copy(value, secret.name)} variant="outline">
                  {copied === secret.name ? "Copied" : "Copy"}
                </Action>
              )}
            </Stack>

            {isConfirming ? (
              <ConfirmStep
                confirmLabel="Regenerate anyway"
                disabled={busy === secret.name}
                message={secret.consequence}
                onCancel={() => setConfirming(undefined)}
                onConfirm={() => {
                  onRegenerate(secret.name);
                  setConfirming(undefined);
                }}
              />
            ) : (
              <Action
                icon="refresh"
                onPress={() => setConfirming(secret.name)}
                style={{ alignSelf: "flex-start" }}
                tone="danger"
                variant="outline"
              >
                Regenerate
              </Action>
            )}
          </Stack>
        );
      })}
    </Stack>
  );
};

export { SecretsPanel };
