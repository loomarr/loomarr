/** A VAPID public key from the server (URL-safe base64, unpadded) as the bytes `pushManager.subscribe` takes. */
const decodeApplicationServerKey = (value: string) => {
  const padded = `${value.replaceAll("-", "+").replaceAll("_", "/")}${"=".repeat((4 - (value.length % 4)) % 4)}`;
  return Uint8Array.from(atob(padded), (character) => character.charCodeAt(0));
};

export { decodeApplicationServerKey };
