import { Field, Surface } from "@loomarr/design-system";

import { useCommittedDraft } from "../use-committed-draft";
import type { NumberFieldProps } from "./number-field.type";

// NumberField — a number in a design-system Field. While the operator types, the box shows their
// text, not the parsed number: "1." re-rendered from Number("1.") would come back as "1" and eat the
// point. The text is kept only while it still reads as the amount shown: a duration re-expresses
// 1.5 days as 36 hours, and the box must then say 36 beside its new unit.
// The flexible Surface fills its row and lets the box shrink beside a unit; a web View does not
// shrink by default. Its basis stays automatic so that in a column it keeps the box's own height.
const NumberField = ({ amount, onAmount, onBlur, ...field }: NumberFieldProps) => {
  // The amount is the parent's and changes on every keystroke; the draft is only the typed text.
  const { discard, draft, editDraft } = useCommittedDraft<string | undefined>(undefined);
  return (
    <Surface backgroundColor="transparent" borderWidth={0} flexGrow={1} flexShrink={1} minWidth={0}>
      <Field
        {...field}
        keyboardType="decimal-pad"
        onBlur={() => {
          discard();
          onBlur?.();
        }}
        onChangeText={(text) => {
          editDraft(text);
          onAmount(Number(text));
        }}
        value={draft !== undefined && Number(draft) === amount ? draft : String(amount)}
      />
    </Surface>
  );
};

export { NumberField };
