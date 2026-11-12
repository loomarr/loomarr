import type { Intent } from "@loomarr/api/models/intent";
import { intentSchema } from "@loomarr/core/schemas";
import { CHANNEL_TEMPLATES } from "@loomarr/core/templates";
import { Action, Stack, Text } from "@loomarr/design-system";
import { useState } from "react";
import { IntentInput } from "@/components/loomarr/ai/intent-input";
import { useAppForm } from "@/lib/app-form";
import type { IntentFormProps } from "./intent-form.type";

// "" → undefined (omitted), never [] — an empty array is a claim ("nothing required"),
// absence is the truth ("the user didn't say").
const terms = (v: string): string[] | undefined => {
  const list = v
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean);
  return list.length > 0 ? list : undefined;
};

interface IntentDraft {
  description: string;
  era: string;
  tone: string;
  runtime: string;
  maxAcq: string;
  mustInclude: string;
  mustExclude: string;
}

// The schema is the same one packages/core shares with mobile, and its field names
// are the wire's (guarded by intent-contract.test.ts).
const parseIntent = (draft: IntentDraft) =>
  intentSchema.safeParse({
    description: draft.description,
    era: draft.era.trim() || undefined,
    tone: draft.tone.trim() || undefined,
    runtimeTargetMin: draft.runtime ? Number(draft.runtime) : undefined,
    maxAcquisitions: draft.maxAcq ? Number(draft.maxAcq) : undefined,
    mustInclude: terms(draft.mustInclude),
    mustExclude: terms(draft.mustExclude),
  });

const hasDetails = (intent?: Intent): boolean =>
  Boolean(
    intent?.era ||
      intent?.tone ||
      intent?.runtimeTargetMin ||
      intent?.maxAcquisitions ||
      intent?.mustInclude?.length ||
      intent?.mustExclude?.length,
  );

// The hero (§3) plus the constraints §13 calls "intent-writing hints": era, tone,
// runtime target, and an acquisition cap. They stay behind a disclosure because the
// blank-page problem is solved by ONE good sentence, not by a form — but they are here,
// and they now actually reach the server (`runtimeTargetMin` was unreachable until the
// submit body was typed from the domain).
const IntentForm = ({
  initialDescription = "",
  initialIntent,
  onSubmit,
  submitting = false,
}: IntentFormProps) => {
  // Details the viewer already filled in stay open when the form comes back (a submit that failed, a
  // draft kept across the setup trip): closing them hides what the request is about to use (#2746).
  const [refining, setRefining] = useState(() => hasDetails(initialIntent));
  const form = useAppForm({
    defaultValues: {
      description: initialIntent?.description ?? initialDescription,
      era: initialIntent?.era ?? "",
      tone: initialIntent?.tone ?? "",
      runtime: initialIntent?.runtimeTargetMin?.toString() ?? "",
      maxAcq: initialIntent?.maxAcquisitions?.toString() ?? "",
      // mustInclude/mustExclude have been in the shared schema and consumed by the scorer
      // (score.go weights a must-include match) since the beginning — with no way to set
      // them short of hand-crafting an API call. Comma-separated because these are a few
      // titles or terms, not a managed list; a tag editor would be more UI than the job needs.
      mustInclude: initialIntent?.mustInclude?.join(", ") ?? "",
      mustExclude: initialIntent?.mustExclude?.join(", ") ?? "",
    },
    validators: {
      onSubmit: ({ value }) => {
        const parsed = parseIntent(value);
        if (parsed.success) return undefined;
        return parsed.error.issues[0]?.message ?? "Check the channel request and try again.";
      },
    },
    onSubmit: ({ value }) => {
      const parsed = parseIntent(value);
      if (parsed.success) onSubmit(parsed.data as Intent);
    },
  });

  return (
    <Stack gap="4">
      <form.Field name="description">
        {(field) => (
          <IntentInput
            value={field.state.value}
            onValueChange={field.handleChange}
            onSubmit={() => void form.handleSubmit()}
            submitting={submitting}
            templates={CHANNEL_TEMPLATES.map((t) => ({ label: t.label, value: t.description }))}
          />
        )}
      </form.Field>
      <form.Subscribe selector={(state) => state.errorMap.onSubmit}>
        {(validationError) =>
          validationError ? (
            <div role="alert">
              <Text as="p" textRole="compact" tone="danger">
                {validationError}
              </Text>
            </div>
          ) : null
        }
      </form.Subscribe>

      <Action
        layout="text"
        onPress={() => setRefining((v) => !v)}
        style={{ alignSelf: "flex-start" }}
        variant="ghost"
      >
        {refining ? "Hide details" : "Add details"}
      </Action>

      {refining && (
        <div className="grid grid-cols-2 gap-3">
          <form.AppField name="era">
            {(field) => <field.TextField label="Era" placeholder="1990s" />}
          </form.AppField>
          <form.AppField name="tone">
            {(field) => <field.TextField label="Tone" placeholder="high-energy" />}
          </form.AppField>
          <form.AppField name="runtime">
            {(field) => <field.TextField label="Target length (minutes)" placeholder="180" type="number" />}
          </form.AppField>
          <form.AppField name="maxAcq">
            {(field) => <field.TextField label="Maximum new titles" placeholder="10" type="number" />}
          </form.AppField>
          <form.AppField name="mustInclude">
            {(field) => (
              <field.TextField className="col-span-2" label="Must include" placeholder="Point Break, Heat" />
            )}
          </form.AppField>
          <form.AppField name="mustExclude">
            {(field) => (
              <field.TextField
                className="col-span-2"
                label="Must exclude"
                placeholder="anything with clowns"
              />
            )}
          </form.AppField>
        </div>
      )}
    </Stack>
  );
};

export { IntentForm };
