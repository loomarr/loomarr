import { SettingEntryProvenance, SettingResultStatus } from "@loomarr/api";
import { formatRelative, humanizeSettingKey } from "@loomarr/core";
import { Lock, TriangleAlert } from "lucide-react";
import { useState } from "react";
import {
  Badge,
  Checkbox,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui";
import { cn } from "@/lib";
import type { SettingFieldProps } from "./setting-field.type";

// SettingField — one registry key as a form control (config-design §2, §3, §6). The
// same field powers the wizard and Settings; there is no parallel wizard form system.
// Everything it renders is contract data: `kind` picks the control, `enum` fills the
// options, `doc` is the help text, `provenance: env` LOCKS the field with a "set via
// environment" chip (env > database > default, visible provenance), and a secret shows
// its masked tail with replace-only editing (§4 — a stored secret is never echoed).
const inputTypeFor = (kind: string): string => {
  if (kind === "int") return "number";
  if (kind === "url") return "url";
  return "text";
};

const SettingField = ({ entry, value, onChange, result, className }: SettingFieldProps) => {
  const [replacing, setReplacing] = useState(false);
  const id = `setting-${entry.key}`;
  const pinned = entry.provenance === SettingEntryProvenance.env;
  const invalid = result?.status === SettingResultStatus.invalid;
  const describedBy = `${id}-doc`;

  // A stored secret renders as its masked tail until the operator opts into replacing it.
  const secretLocked = entry.secret && entry.set && !replacing;

  const control = () => {
    if (entry.kind === "bool") {
      return (
        <Checkbox
          id={id}
          checked={value === "true"}
          disabled={pinned}
          aria-describedby={describedBy}
          onChange={(e) => onChange(String(e.target.checked))}
        />
      );
    }
    if (entry.kind === "enum") {
      return (
        <Select value={value} disabled={pinned} onValueChange={onChange}>
          <SelectTrigger id={id} aria-describedby={describedBy} aria-invalid={invalid ? "true" : undefined}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {(entry.enum ?? []).map((option) => (
              <SelectItem key={option} value={option}>
                {option}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      );
    }
    return (
      <Input
        id={id}
        type={entry.secret ? "password" : inputTypeFor(entry.kind)}
        value={value}
        disabled={pinned}
        autoComplete={entry.secret ? "new-password" : "off"}
        placeholder={entry.secret && entry.set ? "Enter a new value to replace" : undefined}
        aria-describedby={describedBy}
        aria-invalid={invalid ? "true" : undefined}
        onChange={(e) => onChange(e.target.value)}
      />
    );
  };

  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      <div className="flex items-center gap-2">
        <Label htmlFor={id}>{humanizeSettingKey(entry.key)}</Label>
        {pinned && (
          <Badge className="gap-1">
            <Lock className="size-3" aria-hidden />
            set via environment
          </Badge>
        )}
        {result?.status === SettingResultStatus.pinned && !pinned && (
          <Badge variant="caution">not saved — pinned</Badge>
        )}
      </div>

      {secretLocked ? (
        <div className="flex items-center gap-2">
          <span className="flex h-9 flex-1 items-center rounded-md border border-input px-3 font-mono text-muted-foreground text-sm">
            {entry.preview ?? "stored"}
          </span>
          <button
            type="button"
            onClick={() => {
              setReplacing(true);
              onChange("");
            }}
            className="rounded-md border border-input px-3 py-1.5 text-sm transition-colors hover:bg-accent"
          >
            Replace
          </button>
        </div>
      ) : (
        control()
      )}

      <p id={describedBy} className="text-muted-foreground text-xs">
        {entry.doc}
      </p>

      {entry.caution && (
        <p className="flex items-center gap-1 text-onair-300 text-xs">
          <TriangleAlert className="size-3" aria-hidden />
          The stored value was invalid and has been reset to the default.
        </p>
      )}

      {/* "changed by … · when" (§5 field anatomy). Only for a value a PERSON set: an
          env pin or a built-in default has no author, and inventing one would imply a
          human decision that never happened. */}
      {entry.updatedAt && (
        <p className="text-muted-foreground text-xs">
          {entry.updatedBy ? `Changed by ${entry.updatedBy} · ` : "Changed "}
          {formatRelative(entry.updatedAt)}
        </p>
      )}

      {invalid && result?.problem && (
        <p role="alert" className="text-onair-300 text-xs">
          {result.problem}
        </p>
      )}
    </div>
  );
};

export { SettingField };
