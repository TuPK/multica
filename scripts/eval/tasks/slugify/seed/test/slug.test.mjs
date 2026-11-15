import { test } from "node:test";
import assert from "node:assert/strict";
import { slugify } from "../src/slug.mjs";

test("lowercases and joins words with one hyphen", () => {
  assert.equal(slugify("Hello World"), "hello-world");
});

test("collapses runs of other characters and trims hyphens at the ends", () => {
  assert.equal(slugify("  Hello,   World! "), "hello-world");
  assert.equal(slugify("C++ is great"), "c-is-great");
});

test("keeps digits", () => {
  assert.equal(slugify("Release 2 notes"), "release-2-notes");
});
