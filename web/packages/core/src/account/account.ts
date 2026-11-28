import { getMeUrl } from "@loomarr/api/endpoints/auth";
import type { MeBody } from "@loomarr/api/models/meBody";
import { openEnum, readBody } from "@loomarr/api/mutator";
import { MeResponse } from "@loomarr/api/zod/auth";

type Account = { name: string; role: MeBody["role"] };

interface AccountSource {
  load: (signal: AbortSignal) => Promise<Account>;
}

// A role this client does not know reads as itself, which is not "admin"; the server decides access.
const AccountBody = MeResponse.pick({ name: true }).extend({ role: openEnum(MeResponse.shape.role) });

// Who this device is signed in as: `/v1/auth/me`, read through the device's authenticated fetch.
const createAccountSource = (request: typeof globalThis.fetch): AccountSource => ({
  load: async (signal) => {
    const response = await request(getMeUrl(), { method: "GET", signal });
    if (!response.ok) throw new Error(`Couldn't read the account (${response.status}).`);
    return readBody(response, AccountBody);
  },
});

export type { Account, AccountSource };
export { createAccountSource };
