import { Field, Icon, icons, Surface, semanticSpace, semanticTargets } from "@loomarr/design-system";

import type { SearchFieldProps } from "./search-field.type";

// SearchField — a list filter's text box with the magnifier inside its leading edge (People,
// import, the filler catalog). The design-system Field, with its left padding opened up so the
// glyph sits in front of the text rather than over it. The glyph is decoration: the label names it.
const SearchField = ({ disabled, label, onChangeText, placeholder, value }: SearchFieldProps) => (
  <Surface backgroundColor="transparent" borderWidth={0} position="relative" width="100%">
    <Field
      autoCapitalize="none"
      autoCorrect={false}
      disabled={disabled}
      label={label}
      onChangeText={onChangeText}
      placeholder={placeholder}
      style={{ paddingLeft: semanticSpace.control + 24 }}
      value={value}
    />
    <Surface
      alignItems="center"
      backgroundColor="transparent"
      borderWidth={0}
      bottom={0}
      height={semanticTargets.pointer}
      justifyContent="center"
      left={semanticSpace.control}
      pointerEvents="none"
      position="absolute"
    >
      <Icon decorative glyph={icons.search} size="compact" tone="muted" />
    </Surface>
  </Surface>
);

export { SearchField };
