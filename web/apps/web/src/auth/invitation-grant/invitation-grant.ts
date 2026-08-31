// Invitation grants arrive in the fragment so browsers do not send them to the
// server with the initial document request. Move the bearer into caller-owned
// memory and erase it from browser history before any credential field renders.
// It is intentionally never written to localStorage or sessionStorage (§11).
let heldGrant: string | undefined;

const takeInvitationGrantFromLocation = (): string | undefined => {
  const fragment = window.location.hash.startsWith("#")
    ? window.location.hash.slice(1)
    : window.location.hash;
  if (!fragment) return heldGrant;

  const candidate = new URLSearchParams(fragment).get("grant") ?? "";
  window.history.replaceState(
    window.history.state,
    "",
    `${window.location.pathname}${window.location.search}`,
  );
  heldGrant = /^[a-f0-9]{64}$/i.test(candidate) ? candidate : undefined;
  return heldGrant;
};

const clearInvitationGrant = () => {
  heldGrant = undefined;
};

export { clearInvitationGrant, takeInvitationGrantFromLocation };
