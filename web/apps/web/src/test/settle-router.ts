import type { AnyRouter } from "@tanstack/react-router";
import { waitFor } from "@testing-library/react";
import { afterEach, expect } from "vitest";

// A test can end while its router is still mid-transition: a Close or Escape navigates, and the
// next route's load resolves after the test returns. When that test is the file's last, the
// transition lands after jsdom is torn down and Vitest reports `ReferenceError: window is not
// defined` as an unhandled rejection, failing a green shard. Registering a router here makes each
// test wait for it to go idle before Testing Library unmounts it (this hook runs before setup.ts's
// cleanup: after-hooks run in reverse order of registration).

const routers = new Set<AnyRouter>();

const settleOnTeardown = <R extends AnyRouter>(router: R): R => {
  routers.add(router);
  return router;
};

afterEach(async () => {
  const pending = [...routers];
  routers.clear();
  await Promise.all(pending.map((router) => waitFor(() => expect(router.state.status).toBe("idle"))));
});

export { settleOnTeardown };
