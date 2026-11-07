import { type Account, createAccountSource } from "@loomarr/core/account";
import { createAuthenticatedFetch } from "@loomarr/core/pairing";
import { useEffect, useMemo, useState } from "react";

import type { PairedAccountOptions } from "./paired-account.type";

/**
 * Who this paired device is signed in as, for the Device sheet. `undefined` until the server answers,
 * and again if it can't: the sheet then leaves the name and role out rather than guessing them.
 */
const usePairedAccount = ({ credential, session }: PairedAccountOptions): Account | undefined => {
  const [account, setAccount] = useState<Account>();
  const source = useMemo(
    () => createAccountSource(createAuthenticatedFetch(credential, () => session.revoked())),
    [credential, session],
  );
  useEffect(() => {
    const pending = new AbortController();
    source
      .load(pending.signal)
      .then((loaded) => {
        if (!pending.signal.aborted) setAccount(loaded);
      })
      .catch(() => {
        if (!pending.signal.aborted) setAccount(undefined);
      });
    return () => pending.abort();
  }, [source]);
  return account;
};

export { usePairedAccount };
