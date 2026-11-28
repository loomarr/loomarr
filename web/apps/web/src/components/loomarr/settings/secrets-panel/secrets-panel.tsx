import { Action, Card, Stack, Text, typography } from "@loomarr/design-system";
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
          <Card borderRadius={6} gap={8} key={secret.name} padding={12} render="li">
            <div>
              <Text as="p" textRole="cardLabel">
                {secret.label}
              </Text>
              <Text as="p" textRole="compact" tone="secondary">
                {secret.purpose}
              </Text>
            </div>

            <Stack align="center" direction="row" gap="inline">
              <Stack
                backgroundColor="$surfaceElevated"
                borderColor="$borderControl"
                borderRadius={6}
                borderWidth={1}
                flexBasis="0%"
                flexGrow={1}
                flexShrink={1}
                minWidth={0}
                paddingHorizontal={12}
                paddingVertical={6}
              >
                <Text
                  as="code"
                  numberOfLines={1}
                  style={{ fontFamily: typography.family.data.web }}
                  textRole="compact"
                  tone="primary"
                >
                  {value ?? "••••••••••••••••"}
                </Text>
              </Stack>
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
                alignSelf="flex-start"
                tone="danger"
                variant="outline"
              >
                Regenerate
              </Action>
            )}
          </Card>
        );
      })}
    </Stack>
  );
};

export { SecretsPanel };
