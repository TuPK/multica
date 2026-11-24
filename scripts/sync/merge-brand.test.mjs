import assert from "node:assert/strict";
import { test } from "node:test";
import { makeRepo } from "./testlib.mjs";

const ATTRS = "*.md merge=brand\n*.bin merge=brand\n";

function scenario(base, fork, upstream, path = "doc.md") {
  const r = makeRepo(ATTRS);
  r.write(path, base);
  r.commit("base");
  r.git("branch", "upstream");
  r.write(path, fork);
  r.commit("fork");
  r.git("checkout", "-q", "upstream");
  r.write(path, upstream);
  r.commit("upstream");
  r.git("checkout", "-q", "main");
  const conflicts = r.merge("upstream");
  return { r, conflicts, text: r.read(path) };
}

test("fork keeps a deliberate Multica while upstream edits elsewhere", () => {
  const { r, conflicts, text } = scenario(
    "# Multica\nlicence: Multica holder\nnew line\n",
    "# Delegora\nlicence: Multica holder\nnew line\n",
    "# Multica\nlicence: Multica holder\nnew line\nsecond line\n",
  );
  assert.deepEqual(conflicts, []);
  assert.equal(text, "# Delegora\nlicence: Multica holder\nnew line\nsecond line\n");
  r.cleanup();
});

test("a line added by upstream is branded", () => {
  const { r, conflicts, text } = scenario(
    "top\na\n",
    "top fork\na\n",
    "top\na\nWelcome to Multica, run `multica login` (see @multica/core).\n",
  );
  assert.deepEqual(conflicts, []);
  assert.equal(text, "top fork\na\nWelcome to Delegora, run `multica login` (see @multica/core).\n");
  r.cleanup();
});

test("brand-only edits on the fork side never conflict with an upstream edit of the same line", () => {
  const { r, conflicts, text } = scenario(
    "Multica is great\nother\n",
    "Delegora is great\nother\n",
    "Multica is very great\nother\n",
  );
  assert.deepEqual(conflicts, []);
  assert.equal(text, "Delegora is very great\nother\n");
  r.cleanup();
});

test("both sides really editing the same line is a conflict", () => {
  const { r, conflicts } = scenario("x one\n", "x two\n", "x three\n");
  assert.deepEqual(conflicts, ["doc.md"]);
  assert.match(r.read("doc.md"), /<<<<<<< ours/);
  r.cleanup();
});

test("repeated identical lines map one to one", () => {
  const { r, conflicts, text } = scenario(
    "Multica\nMultica\nend\n",
    "Delegora\nMultica\nend\n",
    "Multica\nMultica\nend\nMultica again\n",
  );
  assert.deepEqual(conflicts, []);
  assert.equal(text, "Delegora\nMultica\nend\nDelegora again\n");
  r.cleanup();
});

test("a file with NUL bytes is left to git as a conflict", () => {
  const { r, conflicts } = scenario("a\0b\n", "a\0c\n", "a\0d\n", "blob.bin");
  assert.deepEqual(conflicts, ["blob.bin"]);
  r.cleanup();
});
