import { Badge, Combobox, Icon, icons, Stack, Text } from "@loomarr/design-system";
import { useEffect, useId, useMemo, useState } from "react";
import { languageName } from "@/lib/languages";
import type { LanguagePickerProps } from "./language-picker.type";

// One friendly installation-language control. Provider and model choices stay in Advanced;
// this asks only the household question needed by the automatic gate.
const LanguagePicker = ({ value, options, onChange, locked = false, lockedLabel }: LanguagePickerProps) => {
  const listID = useId();
  const selectedLabel = languageName(value);
  const [query, setQuery] = useState(selectedLabel);

  useEffect(() => setQuery(selectedLabel), [selectedLabel]);

  const choices = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase();
    const localized = options.map((code) => ({ code, label: languageName(code) }));
    if (!needle || query === selectedLabel) return localized;
    return localized.filter(
      ({ code, label }) => label.toLocaleLowerCase().includes(needle) || code.toLocaleLowerCase() === needle,
    );
  }, [options, query, selectedLabel]);

  const choose = (code: string, label: string) => {
    onChange(code);
    setQuery(label);
  };

  return (
    <Stack maxWidth={672}>
      <Stack align="center" direction="row" gap="inline" marginBottom={6}>
        <Text as="label" htmlFor={`${listID}-input`} textRole="cardLabel" tone="primary">
          Commercial language
        </Text>
        {locked && lockedLabel && (
          <Badge
            icon={<Icon decorative glyph={icons.locked} size="compact" tone="inherit" />}
            shape="square"
            tone="neutral"
          >
            {lockedLabel}
          </Badge>
        )}
      </Stack>
      <Combobox
        autoHighlight
        disabled={locked}
        emptyText="No languages found."
        id={`${listID}-input`}
        inputValue={query}
        label="Commercial language"
        onInputValueChange={setQuery}
        onSelect={(option) => choose(option.value, option.label)}
        options={choices.map(({ code, label }) => ({ label, value: code }))}
        placeholder="Search languages"
        value={value}
      />
      <Stack marginTop={8}>
        <Text as="p" textRole="compact" tone="secondary">
          Loomarr skips commercials confidently spoken in another language. Wordless clips stay.
        </Text>
      </Stack>
    </Stack>
  );
};

export { LanguagePicker };
