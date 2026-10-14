import type { UserBody } from "@loomarr/api/models/userBody";

// Invented people: the repo is public, so no fixture borrows a real person's name (#1592). The
// names keep the alphabetical order the earlier ones had, so a sorted roster keeps its order.
const person = (overrides: Partial<UserBody> = {}): UserBody => ({
  id: "person-adela",
  name: "Adela Hale",
  role: "admin",
  quota: 0,
  effectiveQuota: 5,
  pendingAcquisitions: 1,
  autoApprove: true,
  disabled: false,
  local: true,
  offlineLogin: false,
  ...overrides,
});

const people = {
  localAdmin: person(),
  importedMember: person({
    id: "person-greta",
    name: "Greta Finch",
    role: "member",
    quota: 5,
    pendingAcquisitions: 2,
    autoApprove: false,
    local: false,
  }),
  offlineReady: person({
    id: "person-kira",
    name: "Kira Brandt",
    role: "member",
    local: false,
    offlineLogin: true,
  }),
  disabled: person({
    id: "person-alder",
    name: "Alder Wren",
    role: "member",
    disabled: true,
  }),
};

export { people, person };
