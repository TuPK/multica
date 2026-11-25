// Feature flags of the durable platform program (docs/specs/durable-platform,
// spec 14.1). The server publishes them to the clients; a flag that is missing
// reads as off, so every durable screen stays hidden until an operator enables it.
export const DURABLE_FLAGS = {
  outcomes: "durable_outcomes",
  effects: "durable_effects",
  principal: "durable_principal",
  seats: "durable_seats",
  open: "durable_open",
} as const;

export type DurableFlag = (typeof DURABLE_FLAGS)[keyof typeof DURABLE_FLAGS];

export const DURABLE_FLAG_LIST: readonly DurableFlag[] = [
  DURABLE_FLAGS.outcomes,
  DURABLE_FLAGS.effects,
  DURABLE_FLAGS.principal,
  DURABLE_FLAGS.seats,
  DURABLE_FLAGS.open,
];
