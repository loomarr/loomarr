import { pluralize } from "@loomarr/core/format";
import { AlertTriangle, Loader2, RotateCcw } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ServiceControlProps } from "./service-control.type";

// ServiceControl — the restart control (§9.2, V13; v2 mock Dashboard § service control).
//
// ⚠ **No "Reload settings" control, though the mock draws one.** Saving settings already
// hot-applies them (`Save = validate → persist → hot-apply`, config-design §246), so a
// Reload button offers to do something the app has already done. Worse, its presence
// implies saving is NOT enough — it would teach an operator to distrust the save they
// just made. The probe half has a home too: the wizard's connection checklist re-runs on
// demand.
//
// ⚠ **The mock's restart copy is stale and is deliberately not ported.** It promises
// "Channels keep playing — Tunarr streams them, not Loomarr", which §9.1 records as false
// once Loomarr owns the encoder: ffmpeg is its child, killed by process group. What
// replaces it is not a list of facts but the ONE consequence that varies per install —
// how many channels actually drop — read live from the server.

const ServiceControl = ({ cost, onRestart, restarting, error, className }: ServiceControlProps) => {
  const [confirming, setConfirming] = useState(false);

  // The only consequence worth a confirm step. Everything else a restart does (sessions
  // survive, scans resume, Tunarr keeps playing) is either invisible or reassuring, and
  // listing it was noise an operator had to read past to find the part that mattered.
  const dropWarning =
    cost.streamingChannels > 0
      ? `${pluralize(cost.streamingChannels, "channel")} Loomarr is streaming will cut out for a few seconds, then come back.`
      : null;

  return (
    <section className={cn("rounded-lg border border-border p-4", className)}>
      <div className="flex items-center gap-3">
        <div className="min-w-0 flex-1">
          <h2 className="font-medium text-sm">Restart Loomarr</h2>
          <p className="mt-0.5 text-muted-foreground text-sm">
            {dropWarning ?? "Sessions survive; nobody has to sign in again."}
          </p>
        </div>
        {cost.available ? (
          <Button
            type="button"
            variant="outline"
            className="border-signal/35 text-signal"
            onClick={() => setConfirming(true)}
            disabled={restarting || confirming}
          >
            <RotateCcw className="size-4" aria-hidden />
            Restart...
          </Button>
        ) : null}
      </div>

      {/* ⚠ Not a hidden button. A build with no restart loop behind it says so, and says
          what to do instead — hiding the control would leave an operator hunting for a
          feature the docs mention. */}
      {!cost.available && (
        <p className="mt-3 rounded-md border border-border border-dashed p-3 text-muted-foreground text-sm">
          This build can't restart itself. Restart the container or service the way you started it.
        </p>
      )}

      {error ? (
        <p className="mt-3 flex items-start gap-2 text-danger text-sm" role="alert">
          <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden />
          {error}
        </p>
      ) : null}

      {confirming && (
        <div className="mt-3 flex items-center gap-2 border-border border-t pt-3">
          <p className="min-w-0 flex-1 text-sm">
            {dropWarning ? `Restart now? ${dropWarning}` : "Restart now?"}
          </p>
          <Button
            type="button"
            onClick={() => {
              setConfirming(false);
              onRestart();
            }}
            disabled={restarting}
          >
            {restarting ? <Loader2 className="size-4 animate-spin" aria-hidden /> : null}
            {restarting ? "Restarting..." : "Restart now"}
          </Button>
          <Button type="button" variant="ghost" onClick={() => setConfirming(false)}>
            Cancel
          </Button>
        </div>
      )}
    </section>
  );
};

export { ServiceControl };
