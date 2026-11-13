import * as discoveryApi from "@loomarr/api/endpoints/discovery";
import type { DiscoveryFeedbackDTO } from "@loomarr/api/models/discoveryFeedbackDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { Action, Stack } from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";

type DiscoveryFeedbackAction = "keep" | "less" | "never" | "surprise";
type DiscoveryFeedbackScope = { scope: "household" } | { scope: "channel"; scopeId: string };

// The API owns precedence. This hook deliberately indexes the effective projection it
// receives instead of replaying the append-only event history in the browser. Both UI
// surfaces therefore agree with the ranker about which household/channel signal wins.
const useDiscoveryFeedback = (scope: DiscoveryFeedbackScope) => {
  const queryClient = useQueryClient();
  const params = scope.scope === "channel" ? scope : { scope: "household" as const };
  const queryKey = discoveryApi.getListDiscoveryFeedbackQueryKey(params);
  const query = discoveryApi.useListDiscoveryFeedback(params, { query: { retry: false } });

  const invalidate = () => queryClient.invalidateQueries({ queryKey });
  const record = discoveryApi.useRecordDiscoveryFeedback({ mutation: { onSuccess: invalidate } });
  const clear = discoveryApi.useClearDiscoveryFeedback({ mutation: { onSuccess: invalidate } });

  const effective = new Map((unwrap(query.data) ?? []).map((event) => [event.targetKey, event] as const));

  return {
    feedbackFor: (targetKey: string): DiscoveryFeedbackDTO | undefined => effective.get(targetKey),
    setFeedback: (targetKey: string, action: DiscoveryFeedbackAction) =>
      record.mutate({ data: { ...scope, targetKey, action } }),
    clearFeedback: (targetKey: string) => clear.mutate({ data: { ...scope, targetKey } }),
    isPending: query.isPending || record.isPending || clear.isPending,
    error: query.error ?? record.error ?? clear.error,
    retry: () => query.refetch(),
  };
};

const actions: ReadonlyArray<{ action: DiscoveryFeedbackAction; label: string }> = [
  { action: "keep", label: "Keep" },
  { action: "less", label: "Less like this" },
  { action: "never", label: "Never" },
  { action: "surprise", label: "Surprise me" },
];

interface DiscoveryFeedbackControlsProps {
  name: string;
  scope: DiscoveryFeedbackScope;
  effective?: DiscoveryFeedbackDTO;
  disabled?: boolean;
  compact?: boolean;
  // A list that states "affects future suggestions only" once, above its rows, turns the per-row
  // copy off; the row still says which choice is in effect.
  effectNote?: boolean;
  onSet: (action: DiscoveryFeedbackAction) => void;
  onClear: () => void;
}

const actionLabel = (action: string) => actions.find((candidate) => candidate.action === action)?.label;

// One control for both proposal review and Channel programming. It renders the API's
// effective event (including its source scope), while writes remain append-only replace/
// clear commands. An inherited household choice can be overridden here but not erased
// from a Channel, so Undo is intentionally absent in that state.
const DiscoveryFeedbackControls = ({
  name,
  scope,
  effective,
  disabled,
  compact,
  effectNote = true,
  onSet,
  onClear,
}: DiscoveryFeedbackControlsProps) => {
  const selected = actionLabel(effective?.action ?? "") ? effective?.action : undefined;
  const inherited = scope.scope === "channel" && effective?.scope === "household";
  const canClear = effective != null && !inherited;
  const source = inherited
    ? "Inherited household preference"
    : scope.scope === "channel"
      ? "This Channel"
      : "Household";

  return (
    <Stack align={compact ? "end" : "start"} gap="1">
      <Stack direction="row" gap="1" render="fieldset" wrap>
        <legend className="sr-only">Taste feedback for {name}</legend>
        {actions.map(({ action, label }) => (
          <Action
            key={action}
            accessibilityLabel={`${label} — ${name}`}
            aria-pressed={selected === action}
            disabled={disabled}
            onPress={() => onSet(action)}
            // Flat until chosen, so four of them fit beside a title in a lineup row.
            variant={selected === action ? "outline" : "ghost"}
          >
            {compact && action === "less" ? "Less" : compact && action === "surprise" ? "Surprise" : label}
          </Action>
        ))}
        {canClear && (
          <Action disabled={disabled} onPress={onClear} variant="ghost">
            Undo
          </Action>
        )}
      </Stack>
      {(selected || effectNote) && (
        <p className="text-muted-foreground text-xs">
          {selected ? `${source}: ${actionLabel(selected) ?? selected}. ` : ""}
          {effectNote ? "Affects future suggestions only." : ""}
        </p>
      )}
    </Stack>
  );
};

export type { DiscoveryFeedbackAction, DiscoveryFeedbackControlsProps, DiscoveryFeedbackScope };
export { DiscoveryFeedbackControls, useDiscoveryFeedback };
