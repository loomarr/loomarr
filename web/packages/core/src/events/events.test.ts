import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { invalidateByPrefix } from "./events";

describe("invalidateByPrefix", () => {
  it("invalidates only queries whose URL key matches the prefix", async () => {
    const qc = new QueryClient();
    // Seed three cached queries keyed like orval's fetch client ([url, params]).
    await qc.prefetchQuery({ queryKey: ["/v1/titles", { state: "wanted" }], queryFn: () => "t" });
    await qc.prefetchQuery({ queryKey: ["/v1/channels"], queryFn: () => "c" });

    invalidateByPrefix(qc, "/v1/titles");

    const titles = qc.getQueryState(["/v1/titles", { state: "wanted" }]);
    const channels = qc.getQueryState(["/v1/channels"]);
    expect(titles?.isInvalidated).toBe(true);
    expect(channels?.isInvalidated).toBe(false);
  });
});
