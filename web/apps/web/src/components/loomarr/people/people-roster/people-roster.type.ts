import type { UserBody } from "@loomarr/api/models/userBody";

interface PeopleRosterProps {
  users?: UserBody[];
  selectedId?: string;
  selfId?: string;
  onSelect: (user: UserBody) => void;
}

export type { PeopleRosterProps };
