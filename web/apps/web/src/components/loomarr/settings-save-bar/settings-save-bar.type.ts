interface SettingsSaveBarProps {
  // How many keys the operator has changed but not yet saved.
  dirtyCount: number;
  onSave: () => void;
  onDiscard: () => void;
  saving?: boolean;
  className?: string;
}

export type { SettingsSaveBarProps };
