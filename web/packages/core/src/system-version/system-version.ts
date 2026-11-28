import { getSystemVersionUrl } from "@loomarr/api/endpoints/system";
import { readBody } from "@loomarr/api/mutator";
import { SystemVersionResponse } from "@loomarr/api/zod/system";

interface ServerVersionSource {
  load: (signal: AbortSignal) => Promise<string>;
}

const VersionBody = SystemVersionResponse.pick({ dirty: true, version: true });

const createServerVersionSource = (request: typeof globalThis.fetch): ServerVersionSource => ({
  load: async (signal) => {
    const response = await request(getSystemVersionUrl(), { method: "GET", signal });
    if (!response.ok) throw new Error(`Couldn't read the server version (${response.status}).`);
    const version = await readBody(response, VersionBody);
    return `${version.version}${version.dirty ? " (modified)" : ""}`;
  },
});

export type { ServerVersionSource };
export { createServerVersionSource };
