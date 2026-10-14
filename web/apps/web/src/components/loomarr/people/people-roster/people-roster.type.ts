import type { UserBody } from "@loomarr/api/models/userBody";

interface PeopleRosterProps {
  users?: UserBody[];
  selectedId?: string;
  selfId?: string;
  onSelect: (user: UserBody) => void;
  // The clock "Last seen" is relative to; stories freeze it so snapshots don't drift.
  now?: number;
}

export type { PeopleRosterProps };
