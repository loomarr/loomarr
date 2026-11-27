import { Field, Surface } from "@loomarr/design-system";

import { useCommittedDraft } from "../use-committed-draft";
import type { CommitFieldProps } from "./commit-field.type";

// CommitField — a design-system Field that edits a draft and hands it over only on blur, for a
// value whose every keystroke should not save (a policy override, a country code). It keeps its own
// text, so the caller keys it on the stored value to reseed it after a reset.
//
// A draft `validate` refuses is not handed over: it stays in the box with the reason under it, as
// the box's error, until the next edit.
const CommitField = ({ initial, onCommit, validate, width, ...field }: CommitFieldProps) => {
  const { commit, draft, editDraft, problem } = useCommittedDraft(initial, validate);
  return (
    <Surface backgroundColor="transparent" borderWidth={0} width={width}>
      <Field
        autoCorrect={false}
        {...field}
        error={problem}
        onBlur={() => commit(onCommit)}
        onChangeText={editDraft}
        value={draft}
      />
    </Surface>
  );
};

export { CommitField };
