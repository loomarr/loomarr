import { Copy, Eye, EyeOff, Loader2, RefreshCw } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
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
          <li key={secret.name} className="flex flex-col gap-2 rounded-md border border-border bg-card p-3">
            <div>
              <p className="font-medium text-sm">{secret.label}</p>
              <p className="text-muted-foreground text-sm">{secret.purpose}</p>
            </div>

            <div className="flex items-center gap-2">
              <code className="flex-1 truncate rounded-md border border-input bg-static-800 px-3 py-1.5 font-mono text-sm">
                {value ?? "••••••••••••••••"}
              </code>
              <Button
                variant="outline"
                size="sm"
                onClick={() => onReveal(secret.name)}
                disabled={busy === secret.name}
              >
                {value ? <EyeOff className="size-4" aria-hidden /> : <Eye className="size-4" aria-hidden />}
                {value ? "Hide" : "Reveal"}
              </Button>
              {value && (
                <Button variant="outline" size="sm" onClick={() => copy(value, secret.name)}>
                  <Copy className="size-4" aria-hidden />
                  {copied === secret.name ? "Copied" : "Copy"}
                </Button>
              )}
            </div>

            {isConfirming ? (
              <div className="flex flex-col gap-2 rounded-md border border-onair-tint-30 bg-onair-tint-8 p-3">
                <p className="text-onair-300 text-sm">{secret.consequence}</p>
                <div className="flex items-center gap-2">
                  <Button
                    variant="destructive"
                    size="sm"
                    disabled={busy === secret.name}
                    onClick={() => {
                      onRegenerate(secret.name);
                      setConfirming(undefined);
                    }}
                  >
                    {busy === secret.name && <Loader2 className="animate-spin" aria-hidden />}
                    Regenerate anyway
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => setConfirming(undefined)}>
                    Cancel
                  </Button>
                </div>
              </div>
            ) : (
              <Button
                variant="outline"
                size="sm"
                className="w-fit"
                onClick={() => setConfirming(secret.name)}
              >
                <RefreshCw className="size-4" aria-hidden />
                Regenerate
              </Button>
            )}
          </li>
        );
      })}
    </ul>
  );
};

export { SecretsPanel };
