import { settingsApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import type { SecretName, SecretRow } from "@/components/loomarr";
import { ErrorState, SecretsPanel } from "@/components/loomarr";

// The generated-secrets panel (config-design §4/§5). Consequences are spelled out per
// secret because they differ sharply: one breaks integrations, one signs everybody out.
const SECRETS: SecretRow[] = [
  {
    name: "api_token",
    label: "API token",
    purpose: "Break-glass admin access for scripts and automation.",
    consequence:
      "The current token stops working immediately. Anything using it — scripts, automation — must be updated with the new one.",
    displayable: true,
  },
  {
    name: "session_secret",
    label: "Session secret",
    purpose: "Signs session cookies. There is nothing to paste anywhere, so it is never displayed.",
    consequence:
      "Every session is revoked, including yours — you will be signed out immediately. The API token still works as break-glass, so you cannot lock yourself out.",
    displayable: false,
  },
];

const SecretsSettings = () => {
  const [revealed, setRevealed] = useState<Partial<Record<SecretName, string>>>({});
  const [busy, setBusy] = useState<SecretName | undefined>();
  const [error, setError] = useState<unknown>();

  const queryClient = useQueryClient();
  const regenerate = settingsApi.useSecretRegenerate();

  const onReveal = (name: SecretName) => {
    // A second click hides it again — the eye toggle (§4), not a one-way door.
    if (revealed[name]) {
      setRevealed((p) => ({ ...p, [name]: undefined }));
      return;
    }
    // Revealing is a GET, so it is fetched imperatively on click rather than mounted as
    // a live query — a secret should cross the wire when the operator asks for it, not
    // sit in the cache of every page that renders this panel.
    setBusy(name);
    queryClient
      .fetchQuery(settingsApi.getSecretRevealQueryOptions(name))
      .then((res) => {
        if (res.status === 200 && res.data.displayable) {
          setRevealed((p) => ({ ...p, [name]: res.data.value }));
        }
      })
      .catch(setError)
      .finally(() => setBusy(undefined));
  };

  const onRegenerate = (name: SecretName) => {
    setBusy(name);
    regenerate.mutate(
      { name },
      {
        onSuccess: (res) => {
          // Show the new value straight away for the ones you must paste elsewhere —
          // this is the one moment the operator can copy it without another round trip.
          if (res.status === 200 && res.data.displayable && res.data.value) {
            setRevealed((p) => ({ ...p, [name]: res.data.value }));
          }
          setBusy(undefined);
        },
        onError: (e) => {
          setError(e);
          setBusy(undefined);
        },
      },
    );
  };

  return (
    <div className="flex flex-col gap-3">
      {error != null && <ErrorState error={error} />}
      <SecretsPanel
        secrets={SECRETS}
        revealed={revealed}
        onReveal={onReveal}
        onRegenerate={onRegenerate}
        busy={busy}
      />
    </div>
  );
};

export { SecretsSettings };
