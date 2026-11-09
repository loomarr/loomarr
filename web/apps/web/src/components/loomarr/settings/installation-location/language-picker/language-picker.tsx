import { Field, Icon, icons } from "@loomarr/design-system";
import { useEffect, useId, useMemo, useState } from "react";
import { languageName } from "@/lib/languages";
import { cn } from "@/lib/utils";
import type { LanguagePickerProps } from "./language-picker.type";

// One friendly installation-language control. Provider and model choices stay in Advanced;
// this asks only the household question needed by the automatic gate.
const LanguagePicker = ({ value, options, onChange, locked = false, lockedLabel }: LanguagePickerProps) => {
  const listID = useId();
  const selectedLabel = languageName(value);
  const [query, setQuery] = useState(selectedLabel);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);

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
    setOpen(false);
    setActive(0);
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
      <div className="relative">
        <Field
          aria-activedescendant={open && choices[active] ? `${listID}-${active}` : undefined}
          aria-autocomplete="list"
          aria-controls={listID}
          aria-expanded={open}
          autoComplete="off"
          autoCorrect={false}
          disabled={locked}
          id={`${listID}-input`}
          onBlur={() => setOpen(false)}
          onChangeText={(text) => {
            setQuery(text);
            setOpen(true);
            setActive(0);
          }}
          onFocus={() => setOpen(true)}
          onKeyPress={(event) => {
            const key = event.nativeEvent.key;
            if (!open || choices.length === 0) return;
            if (key === "ArrowDown") {
              event.preventDefault();
              setActive((current) => Math.min(current + 1, choices.length - 1));
            } else if (key === "ArrowUp") {
              event.preventDefault();
              setActive((current) => Math.max(current - 1, 0));
            } else if (key === "Enter" && choices[active]) {
              event.preventDefault();
              choose(choices[active].code, choices[active].label);
            } else if (key === "Escape") {
              setOpen(false);
            }
          }}
          placeholder="Search languages"
          role="combobox"
          value={query}
        />
        {open && !locked && (
          <div
            id={listID}
            role="listbox"
            className="border-border bg-popover text-popover-foreground relative z-20 mt-1 max-h-64 w-full overflow-auto rounded-md border p-1 shadow-lg sm:absolute"
          >
            {choices.map(({ code, label }, index) => (
              <button
                type="button"
                id={`${listID}-${index}`}
                key={code || "any"}
                role="option"
                aria-selected={code === value}
                tabIndex={-1}
                className={cn(
                  "flex w-full cursor-pointer items-center justify-between rounded-sm px-3 py-2 text-left text-sm",
                  index === active ? "bg-accent text-accent-foreground" : "hover:bg-accent/70",
                )}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => choose(code, label)}
              >
                <span>{label}</span>
                {code === value && <Icon decorative glyph={icons.selected} size="compact" tone="inherit" />}
              </button>
            ))}
            {choices.length === 0 && (
              <p className="text-muted-foreground px-3 py-3 text-sm">No languages found.</p>
            )}
          </div>
        )}
      </div>
      <p className="text-muted-foreground mt-2 text-sm">
        Loomarr skips commercials confidently spoken in another language. Wordless clips stay.
      </p>
    </div>
  );
};

export { LanguagePicker };
