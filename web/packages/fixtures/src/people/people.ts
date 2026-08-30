import type { UserBody } from "@loomarr/api/models/userBody";

const person = (overrides: Partial<UserBody> = {}): UserBody => ({
  id: "person-ada",
  name: "Ada Lovelace",
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
    id: "person-grace",
    name: "Grace Hopper",
    role: "member",
    quota: 5,
    pendingAcquisitions: 2,
    autoApprove: false,
    local: false,
  }),
  offlineReady: person({
    id: "person-katherine",
    name: "Katherine Johnson",
    role: "member",
    local: false,
    offlineLogin: true,
  }),
  disabled: person({
    id: "person-alan",
    name: "Alan Turing",
    role: "member",
    disabled: true,
  }),
};

export { people, person };
