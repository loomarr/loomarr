import { Combobox, Icon, icons } from "@loomarr/design-system";
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
    <div className="max-w-2xl">
      <div className="mb-1.5 flex items-center gap-2">
        <label htmlFor={`${listID}-input`} className="text-sm font-medium">
          Commercial language
        </label>
        {locked && lockedLabel && (
          <span className="bg-static-800 text-2xs text-static-400 inline-flex items-center gap-1 rounded-sm px-1.5 py-0.5 font-mono">
            <Icon decorative glyph={icons.locked} size="compact" tone="inherit" /> {lockedLabel}
          </span>
        )}
      </div>
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
      <p className="text-muted-foreground mt-2 text-sm">
        Loomarr skips commercials confidently spoken in another language. Wordless clips stay.
      </p>
    </div>
  );
};

export { LanguagePicker };
