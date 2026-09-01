import * as systemApi from "@loomarr/api/endpoints/system";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";

const EncryptionSettings = () => {
  const queryClient = useQueryClient();
  const status = systemApi.useSystemEncryptionStatus({ query: { retry: false } });
  const rotate = systemApi.useSystemEncryptionRotateDataKey();
  const [message, setMessage] = useState<string>();
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
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="font-medium text-sm">Database secret encryption</h2>
          <p className="mt-1 max-w-2xl text-muted-foreground text-sm leading-relaxed">
            Credentials saved in Loomarr are encrypted before they enter the database. Keep the installation
            key with your host or secrets manager—it is not included in database backups.
          </p>
        </div>
        <Button disabled={!encryption?.enabled || rotate.isPending} onClick={onRotate} variant="outline">
          {rotate.isPending ? "Rotating…" : "Rotate data key"}
        </Button>
      </div>
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
            <dd className="break-all font-mono text-xs">
              {encryption.installationKeyFingerprint || "Unavailable"}
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Readable data keys</dt>
            <dd className="font-medium">{encryption.dataKeyCount}</dd>
          </div>
        </dl>
      )}
      {message != null && <p className="mt-3 text-muted-foreground text-sm">{message}</p>}
    </section>
  );
};

export { EncryptionSettings };
