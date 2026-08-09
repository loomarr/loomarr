import type { ChannelDTO } from "@loomarr/api";

// A minimal VALID ChannelDTO — every required field the wire declares, nothing more.
//
// ⚠ It exists because "minimal" and "valid" stopped being the same thing. Tests used to invent
// `{ id, name, status }` inline and hand it back as a channel; the wire requires eleven fields
// (`breakCount`, `lineup`, `number`, `pendingCount`, `policy`, `programCount`, `slotCount`,
// `strategy`, …). A hand-rolled fetch stub is untyped, so those objects passed for as long as
// nobody typed them — and a component reading `pendingCount` off one would have seen `undefined`
// where the server always sends a number.
//
// Spread it and override only what a test actually asserts on:
//
//   channel({ status: "paused" })
//
// ⚠ Keep this HONEST rather than convenient: if the wire gains a required field, this file should
// fail to typecheck and be updated — that failure is the fixture layer doing its job. Do not
// silence it with `as ChannelDTO`.
const channel = (over: Partial<ChannelDTO> = {}): ChannelDTO => ({
  id: "ch-1",
  name: "Late Night Noir",
  number: 1,
  status: "live",
  strategy: "shuffle",
  lineup: [],
  policy: {},
  breakCount: 0,
  pendingCount: 0,
  programCount: 0,
  slotCount: 0,
  ...over,
});

export { channel };
