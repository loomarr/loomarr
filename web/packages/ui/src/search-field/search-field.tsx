import { Field, Icon, icons, Surface, semanticSpace, semanticTargets } from "@loomarr/design-system";

import type { SearchFieldProps } from "./search-field.type";

// SearchField — a list filter's text box with the magnifier inside its leading edge (People,
// import, Settings, the filler catalog). The design-system Field, with its left padding opened up so
// the glyph sits in front of the text rather than over it. The glyph is decoration: the label names
// it, printed above the box or, with `labelHidden`, only announced.
const SearchField = ({
  disabled,
  label,
  labelHidden = false,
  onChangeText,
  onKeyPress,
  placeholder,
  value,
}: SearchFieldProps) => (
  <Surface backgroundColor="transparent" borderWidth={0} position="relative" width="100%">
    <Field
      accessibilityLabel={label}
      autoCapitalize="none"
      autoCorrect={false}
      disabled={disabled}
      label={labelHidden ? undefined : label}
      onChangeText={onChangeText}
      onKeyPress={onKeyPress}
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
