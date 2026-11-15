import * as systemApi from "@loomarr/api/endpoints/system";
import { Action, Grid, Stack, Text } from "@loomarr/design-system";
import { ConfirmStep } from "@loomarr/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";

const EncryptionSettings = () => {
  const queryClient = useQueryClient();
  const status = systemApi.useSystemEncryptionStatus({ query: { retry: false } });
  const rotate = systemApi.useSystemEncryptionRotateDataKey();
  const [message, setMessage] = useState<string>();
  // Rotation can't be taken back, so the first press only asks, as Delete channel does.
  const [confirming, setConfirming] = useState(false);
  const encryption = status.data?.status === 200 ? status.data.data : undefined;

  const onRotate = () => {
    setMessage(undefined);
    rotate.mutate(undefined, {
      onSuccess: async () => {
        await queryClient.invalidateQueries({ queryKey: systemApi.getSystemEncryptionStatusQueryKey() });
        setMessage(
          "Data-encryption key rotated. Stored settings were re-encrypted, and new secret writes use the new key.",
        );
      },
    });
  };

  return (
    <Stack borderColor="$borderDecorative" borderRadius={8} borderWidth={1} padding={16} render="section">
      <Stack align="start" direction="row" gap="4" justify="between" wrap>
        <div>
          <Text as="h2" textRole="cardLabel">
            Database secret encryption
          </Text>
          <Stack marginTop={4} maxWidth={672}>
            <Text as="p" textRole="compact" tone="secondary">
              Credentials saved in Loomarr are encrypted before they enter the database. Keep the installation
              key with your host or secrets manager—it is not included in database backups.
            </Text>
          </Stack>
        </div>
        {confirming ? (
          <ConfirmStep
            confirmLabel="Rotate data key"
            disabled={rotate.isPending}
            message="Rotate the data key? Every stored setting is re-encrypted with a new key. This can't be undone."
            onCancel={() => setConfirming(false)}
            onConfirm={() => {
              setConfirming(false);
              onRotate();
            }}
          />
        ) : (
          <Action
            disabled={!encryption?.enabled || rotate.isPending}
            onPress={() => setConfirming(true)}
            tone="danger"
            variant="outline"
          >
            {rotate.isPending ? "Rotating…" : "Rotate data key"}
          </Action>
        )}
      </Stack>
      {status.error != null && (
        <Stack marginTop={12}>
          <ErrorState error={status.error} />
        </Stack>
      )}
      {rotate.error != null && (
        <Stack marginTop={12}>
          <ErrorState error={rotate.error} />
        </Stack>
      )}
      {encryption != null && (
        <Grid columns={{ base: 1, sm: 3 }} gap="3" marginTop={16} render="dl">
          <div>
            <Text as="dt" textRole="compact" tone="secondary">
              Status
            </Text>
            <Text as="dd" textRole="cardLabel">
              {encryption.enabled ? "Encrypted" : "Unavailable"}
            </Text>
          </div>
          <div>
            <Text as="dt" textRole="compact" tone="secondary">
              Installation key fingerprint
            </Text>
            <Text as="dd" className="break-all" textRole="mono" tone="primary">
              {encryption.installationKeyFingerprint || "Unavailable"}
            </Text>
          </div>
          <div>
            <Text as="dt" textRole="compact" tone="secondary">
              Readable data keys
            </Text>
            <Text as="dd" textRole="cardLabel">
              {encryption.dataKeyCount}
            </Text>
          </div>
        </Grid>
      )}
      {message != null && (
        <Stack marginTop={12}>
          <Text as="p" textRole="compact" tone="secondary">
            {message}
          </Text>
        </Stack>
      )}
    </Stack>
  );
};

export { EncryptionSettings };
