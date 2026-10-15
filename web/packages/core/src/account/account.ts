import { getMeUrl } from "@loomarr/api/endpoints/auth";
import type { MeBody } from "@loomarr/api/models/meBody";

type Account = { name: string; role: MeBody["role"] };

interface AccountSource {
  load: (signal: AbortSignal) => Promise<Account>;
}

// Who this device is signed in as: `/v1/auth/me`, read through the device's authenticated fetch.
const createAccountSource = (request: typeof globalThis.fetch): AccountSource => ({
  load: async (signal) => {
    const response = await request(getMeUrl(), { method: "GET", signal });
    if (!response.ok) throw new Error(`Couldn't read the account (${response.status}).`);
    const me = (await response.json()) as MeBody;
    return { name: me.name, role: me.role };
  },
});

export type { Account, AccountSource };
export { createAccountSource };
