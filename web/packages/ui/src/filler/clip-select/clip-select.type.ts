type ClipSelectProps = {
  /** The clip's name: the control announces "Select <name>" and prints nothing. */
  clipName: string;
  onSelectedChange: (selected: boolean) => void;
  selected: boolean;
};

export type { ClipSelectProps };
