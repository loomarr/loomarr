import { Field, Surface } from "@loomarr/design-system";
import { useState } from "react";

import type { CommitFieldProps } from "./commit-field.type";

// CommitField — a design-system Field that edits a draft and hands it over only on blur, for a
// value whose every keystroke should not save (a policy override, a country code). It keeps its own
// text, so the caller keys it on the stored value to reseed it after a reset.
const CommitField = ({ initial, onCommit, width, ...field }: CommitFieldProps) => {
  const [text, setText] = useState(initial);
  return (
    <Surface backgroundColor="transparent" borderWidth={0} width={width}>
      <Field
        autoCorrect={false}
        {...field}
        onBlur={() => onCommit(text)}
        onChangeText={setText}
        value={text}
      />
    </Surface>
  );
};

export { CommitField };
