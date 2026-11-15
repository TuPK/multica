import { test } from "node:test";
import assert from "node:assert/strict";
import { parseDuration } from "../src/duration.mjs";

test("single units", () => {
  assert.equal(parseDuration("45s"), 45);
  assert.equal(parseDuration("2h"), 7200);
  assert.equal(parseDuration("2d"), 172800);
});

test("combined units add up", () => {
  assert.equal(parseDuration("1h30m"), 5400);
  assert.equal(parseDuration("10m5s"), 605);
});

test("invalid input throws a RangeError", () => {
  assert.throws(() => parseDuration(""), RangeError);
  assert.throws(() => parseDuration("abc"), RangeError);
  assert.throws(() => parseDuration("5w"), RangeError);
});
