import * as systemApi from "@loomarr/api/endpoints/system";
import { Action, Stack } from "@loomarr/design-system";
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
    <section className="border-border rounded-lg border p-4">
      <Stack align="start" direction="row" gap="4" justify="between" wrap>
        <div>
          <h2 className="text-sm font-medium">Database secret encryption</h2>
          <p className="text-muted-foreground mt-1 max-w-2xl text-sm leading-relaxed">
            Credentials saved in Loomarr are encrypted before they enter the database. Keep the installation
            key with your host or secrets manager—it is not included in database backups.
          </p>
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
        <div className="mt-3">
          <ErrorState error={status.error} />
        </div>
      )}
      {rotate.error != null && (
        <div className="mt-3">
          <ErrorState error={rotate.error} />
        </div>
      )}
      {encryption != null && (
        <dl className="mt-4 grid gap-3 text-sm sm:grid-cols-3">
          <div>
            <dt className="text-muted-foreground">Status</dt>
            <dd className="font-medium">{encryption.enabled ? "Encrypted" : "Unavailable"}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Installation key fingerprint</dt>
            <dd className="font-mono text-xs break-all">
              {encryption.installationKeyFingerprint || "Unavailable"}
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Readable data keys</dt>
            <dd className="font-medium">{encryption.dataKeyCount}</dd>
          </div>
        </dl>
      )}
      {message != null && <p className="text-muted-foreground mt-3 text-sm">{message}</p>}
    </section>
  );
};

export { EncryptionSettings };
