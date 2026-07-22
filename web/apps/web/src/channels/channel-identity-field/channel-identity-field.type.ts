type ChannelIdentityFieldProps = {
  // Accessible label (e.g. "Channel name", "Channel number").
  label: string;
  // The committed value (the source of truth from the server). The field seeds its draft
  // from this and resets to it on Cancel / after a successful save.
  value: string | number;
  // "text" for the name, "number" for the channel number — drives the input type + parsing.
  type?: "text" | "number";
  // Validate the draft before Save is allowed. Return an error string to block + explain, or
  // undefined when valid. Runs on every keystroke so Save is disabled for an invalid draft.
  validate?: (draft: string) => string | undefined;
  // Commit the (validated) value. RESOLVES on success → the field resets to it and closes;
  // REJECTS on failure (e.g. a 409 duplicate number) → the field surfaces the reason inline
  // and stays open so the user can fix it. The parent wraps its mutation's mutateAsync.
  onSave: (next: string | number) => Promise<void>;
  // Visual sizing hint: "title" (the big channel name) vs "compact" (the channel number).
  variant?: "title" | "compact";
  className?: string;
};

export type { ChannelIdentityFieldProps };
