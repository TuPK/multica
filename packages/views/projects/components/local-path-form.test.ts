// @vitest-environment node
import { describe, expect, it } from "vitest";
import { isAbsoluteLocalPath } from "./local-path-form";

describe("isAbsoluteLocalPath", () => {
  it.each(["/home/me/repo", "C:\\Users\\me\\repo", "d:/work/repo", "\\\\server\\share"])(
    "accepts %s",
    (path) => {
      expect(isAbsoluteLocalPath(path)).toBe(true);
    },
  );

  it.each(["", "repo", "./repo", "~/repo", "C:repo", "../repo"])(
    "rejects %s",
    (path) => {
      expect(isAbsoluteLocalPath(path)).toBe(false);
    },
  );
});
