import { test } from "node:test";
import assert from "node:assert/strict";
import { countOpen, sumOpen } from "../src/todos.mjs";

test("countOpen counts open items", () => {
  assert.equal(countOpen([{ status: "open" }, { status: "done" }]), 1);
});

test("sumOpen adds the amounts of open items only", () => {
  const list = [
    { status: "open", amount: 5 },
    { status: "done", amount: 100 },
    { status: "open", amount: 7 },
  ];
  assert.equal(sumOpen(list), 12);
});

test("sumOpen of an empty list is 0", () => {
  assert.equal(sumOpen([]), 0);
});
