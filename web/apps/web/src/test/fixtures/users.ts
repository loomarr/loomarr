import type { MeBody } from "@loomarr/api/models/meBody";
import type { UserBody } from "@loomarr/api/models/userBody";

// The two people fixtures. Same reasoning as `./channels` — see that file's header — but this pair
// carries an extra trap worth naming: `MeBody` and `UserBody` LOOK interchangeable and are not.
//
// ⚠ `UserBody` requires `effectiveQuota` and `pendingAcquisitions`; `MeBody` has neither. Tests
// built their users list by spreading the signed-in user — `USERS = [{ ...ADMIN, local: true }]`
// in `test/users` — which produces an object missing both, and an untyped fetch stub served it
// happily. A screen reading `pendingAcquisitions` off one of those sees `undefined` where the
// server always sends a number.
//
// ⚠ `local` is REQUIRED on MeBody and was absent from FIVE hand-rolled fixtures. That is the field
// this file exists to stop re-litigating.
//
// Both default to an ADMIN, following `channel()`'s precedent of defaulting to the common case:
// most screens under test are admin screens. §19's negative cases say so explicitly —
// `me({ role: "member" })` — which is the right way round, because a test asserting a 403 is
// asserting about the role and should name it.
const me = (over: Partial<MeBody> = {}): MeBody => ({
  id: "u1",
  name: "Ada",
  role: "admin",
  local: true,
  autoApprove: true,
  disabled: false,
  quota: 0,
  ...over,
});

const user = (over: Partial<UserBody> = {}): UserBody => ({
  id: "u1",
  name: "Ada",
  role: "admin",
  local: true,
  autoApprove: true,
  disabled: false,
  quota: 0,
  effectiveQuota: 0,
  pendingAcquisitions: 0,
  usageAvailable: true,
  ...over,
});

export { me, user };
