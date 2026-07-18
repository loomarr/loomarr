import { settingsApi, setupApi } from "@loomarr/api";
import { formatRelative } from "@loomarr/core";
import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { ChecklistItem, ErrorState } from "@/components/loomarr";
import { Button } from "@/components/ui";

// The two apps that send Loomarr webhooks (§6). Each reports independently so a
// half-configured install says exactly which one is still missing.
const APPS = ["sonarr", "radarr"] as const;

// Wizard step 4 — the webhook handshake (§13). The operator pastes one URL into Sonarr
// and Radarr, clicks *Test* there, and this screen flips green on receipt: no more
// "did the webhook actually work?" guesswork. The URL carries the generated
// WEBHOOK_SECRET as its token, revealed (never rotated — §4) so what's shown is what
// they already pasted. Polls while the operator is off in the other tab.
const WebhookStep = () => {
  const [copied, setCopied] = useState(false);
  const secret = settingsApi.useSecretReveal("webhook_secret");
  const status = setupApi.useSetupStatus({ query: { refetchInterval: 5000 } });

  const checks = status.data?.status === 200 ? (status.data.data.checks ?? []) : [];
  const webhook = checks.find((c) => c.name === "webhook");
  const lastReceived = webhook?.lastReceived ?? {};

  const token = secret.data?.status === 200 ? (secret.data.data.value ?? "") : "";
  const url = token ? `${window.location.origin}/hooks/arr?token=${token}` : "";

  const copy = () => {
    void navigator.clipboard?.writeText(url);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="flex flex-col gap-4">
      <p className="text-muted-foreground text-sm">
        Paste this URL into Sonarr and Radarr (Settings → Connect → Webhook), then press
        <strong className="text-foreground"> Test</strong> in each. This page turns green the moment it hears
        from them.
      </p>

      {secret.error != null && <ErrorState error={secret.error} onRetry={() => secret.refetch()} />}

      <div className="flex items-center gap-2">
        <code className="flex-1 truncate rounded-md border border-input bg-static-800 px-3 py-2 font-mono text-sm">
          {url || "Loading the webhook URL…"}
        </code>
        <Button variant="outline" onClick={copy} disabled={!url}>
          {copied ? <Check className="size-4" aria-hidden /> : <Copy className="size-4" aria-hidden />}
          {copied ? "Copied" : "Copy"}
        </Button>
      </div>

      <ul className="flex flex-col gap-2">
        {APPS.map((app) => {
          const at = lastReceived[app];
          return (
            <li key={app}>
              {/* ChecklistItem shows a hint only on `fail`, and "still listening" is
                  not a failure — so the per-app status line lives here. */}
              <ChecklistItem name={app === "sonarr" ? "Sonarr" : "Radarr"} status={at ? "pass" : "running"} />
              <p className="mt-0.5 pl-9 text-muted-foreground text-xs">
                {at ? `Last received ${formatRelative(at)}` : "Listening — press Test in this app."}
              </p>
            </li>
          );
        })}
      </ul>

      <p className="text-muted-foreground text-xs">
        Both are optional — Loomarr only needs a webhook from the apps you actually use to know when a
        download finished.
      </p>
    </div>
  );
};

export { WebhookStep };
