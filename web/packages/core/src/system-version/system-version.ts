import { getSystemVersionUrl } from "@loomarr/api/endpoints/system";
import { importSchemas, readBody } from "@loomarr/api/mutator";

interface ServerVersionSource {
  load: (signal: AbortSignal) => Promise<string>;
}

const createServerVersionSource = (request: typeof globalThis.fetch): ServerVersionSource => ({
  load: async (signal) => {
    const response = await request(getSystemVersionUrl(), { method: "GET", signal });
    if (!response.ok) throw new Error(`Couldn't read the server version (${response.status}).`);
    const { VersionBody } = await importSchemas(() => import("../response-bodies"));
    const version = await readBody(response, VersionBody);
    return `${version.version}${version.dirty ? " (modified)" : ""}`;
  },
});

export type { ServerVersionSource };
export { createServerVersionSource };
