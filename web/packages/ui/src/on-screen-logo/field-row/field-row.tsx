import { Badge, FieldHint, Surface, Text } from "@loomarr/design-system";
import { type ReactNode, useState } from "react";
import { Pressable, View } from "react-native";
import type { OnScreenLogoKey } from "../on-screen-logo.type";
import { row } from "../styles";

interface RowProps {
  children: ReactNode;
  field?: OnScreenLogoKey;
  help: string;
  hint?: string;
  label: string;
  off?: boolean;
  onReset?: () => void;
  overridden?: boolean;
}

// One field row (the Programming tab's PolicyField shape): label + "?" help, Default/Channel
// override with Reset to default, the control, then a plain-language hint.
const Row = ({ children, field, help, hint, label, off, onReset, overridden }: RowProps) => {
  const [showHelp, setShowHelp] = useState(false);
  return (
    <View style={{ gap: 8 }} testID={field ? `logo-field-${field}` : undefined}>
      <View style={{ ...row, flexWrap: "wrap", justifyContent: "space-between" }}>
        <View style={{ ...row, gap: 6 }}>
          <Text textRole="label">{label}</Text>
          {/* The mock's .help: an 18 px outlined circle beside the label, so the header stays one row. */}
          <Pressable
            accessibilityLabel={`About ${label}`}
            accessibilityRole="button"
            aria-expanded={showHelp}
            hitSlop={13}
            onPress={() => setShowHelp((v) => !v)}
            // react-native-web ignores hitSlop, so the web's tap area is this box: 24 px around the
            // 18 px circle, drawn where it was (#1785).
            style={{ margin: -3, padding: 3 }}
          >
            <Surface
              alignItems="center"
              backgroundColor="transparent"
              borderColor="borderControl"
              borderRadius="round"
              borderWidth={1}
              height={18}
              justifyContent="center"
              width={18}
            >
              <Text textRole="mono" tone="muted">
                ?
              </Text>
            </Surface>
          </Pressable>
        </View>
        {onReset ? (
          <View style={row}>
            <Badge shape="square" tone={overridden ? "override" : "neutral"}>
              {overridden ? "Channel override" : "Default"}
            </Badge>
            {/* The mock's .link: 12 px underlined text, no button box, so the header stays one row.
                Only beside an override: beside Default it would have nothing to reset (#2744). */}
            {overridden ? (
              <Pressable
                accessibilityLabel={`Reset to default (${label})`}
                accessibilityRole="button"
                hitSlop={12}
                onPress={onReset}
                // As the help circle's: a 24 px tall tap area on the web for 16 px of text (#1785).
                style={{ marginVertical: -4, paddingVertical: 4 }}
              >
                <Text textDecorationLine="underline" textRole="hint" tone="primary">
                  Reset to default
                </Text>
              </Pressable>
            ) : null}
          </View>
        ) : null}
      </View>
      {showHelp ? <FieldHint>{help}</FieldHint> : null}
      {/* Paused rows fade only the control; the label and hint stay at full contrast (AA). */}
      <View style={{ opacity: off ? 0.55 : 1 }}>{children}</View>
      {hint ? <FieldHint>{hint}</FieldHint> : null}
    </View>
  );
};

export { Row };
