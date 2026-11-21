import { pluralize } from "@loomarr/core/format";
import { Action, Icon, icons, Stack, Text } from "@loomarr/design-system";
import { useState } from "react";
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
  // listing it is noise an operator has to read past to find the part that mattered.
  const dropWarning =
    cost.streamingChannels > 0
      ? `${pluralize(cost.streamingChannels, "channel")} Loomarr is streaming will cut out for a few seconds, then come back.`
      : null;

  return (
    <section className={cn("border-border rounded-lg border p-4", className)}>
      <Stack align="center" direction="row" gap="control">
        <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minWidth={0}>
          <Text as="h2" textRole="cardLabel">
            Restart Loomarr
          </Text>
          <Stack marginTop={2}>
            <Text as="p" textRole="compact" tone="secondary">
              {dropWarning ?? "Sessions survive; nobody has to sign in again."}
            </Text>
          </Stack>
        </Stack>
        {cost.available ? (
          <Action
            disabled={restarting || confirming}
            icon="refresh"
            onPress={() => setConfirming(true)}
            variant="outline"
          >
            Restart...
          </Action>
        ) : null}
      </Stack>

      {/* ⚠ Not a hidden button. A build with no restart loop behind it says so, and says
          what to do instead — hiding the control would leave an operator hunting for a
          feature the docs mention. */}
      {!cost.available && (
        <Stack marginTop={12}>
          <Text
            as="p"
            className="border-border rounded-md border border-dashed p-3"
            textRole="compact"
            tone="secondary"
          >
            This build can't restart itself. Restart the container or service the way you started it.
          </Text>
        </Stack>
      )}

      {error ? (
        <Stack marginTop={12} role="alert">
          <Stack align="start" direction="row" gap="inline">
            <Stack direction="row" marginTop={2} render="span">
              <Icon decorative glyph={icons.alert} size="compact" tone="danger" />
            </Stack>
            <Text as="p" textRole="compact" tone="danger">
              {error}
            </Text>
          </Stack>
        </Stack>
      ) : null}

      {confirming && (
        <Stack
          align="center"
          borderColor="borderDecorative"
          borderTopWidth={1}
          direction="row"
          gap="inline"
          marginTop={12}
          paddingTop={12}
        >
          <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minWidth={0}>
            <Text as="p" textRole="compact">
              {dropWarning ? `Restart now? ${dropWarning}` : "Restart now?"}
            </Text>
          </Stack>
          <Action
            disabled={restarting}
            onPress={() => {
              setConfirming(false);
              onRestart();
            }}
          >
            {restarting ? "Restarting..." : "Restart now"}
          </Action>
          <Action onPress={() => setConfirming(false)} variant="ghost">
            Cancel
          </Action>
        </Stack>
      )}
    </section>
  );
};

export { ServiceControl };
