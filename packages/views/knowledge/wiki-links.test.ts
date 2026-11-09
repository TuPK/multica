// @vitest-environment node
import { describe, expect, it } from "vitest";
import { wikiLinksToMarkdown, wikiSlugFromHref } from "./wiki-links";

describe("wikiLinksToMarkdown", () => {
  it("turns labelled and bare wiki links into links", () => {
    expect(wikiLinksToMarkdown("See [[entity/mua-ngon|Mùa Ngon]] and [[concept/trust]].")).toBe(
      "See [Mùa Ngon](#wiki/entity/mua-ngon) and [trust](#wiki/concept/trust).",
    );
  });

  it("leaves text without wiki links alone", () => {
    expect(wikiLinksToMarkdown("a [link](https://x.y) and [ref]")).toBe("a [link](https://x.y) and [ref]");
  });
});

describe("wikiSlugFromHref", () => {
  it("reads the slug back and ignores other hrefs", () => {
    expect(wikiSlugFromHref("#wiki/entity/mua-ngon")).toBe("entity/mua-ngon");
    expect(wikiSlugFromHref("https://x.y")).toBeNull();
    expect(wikiSlugFromHref(undefined)).toBeNull();
  });
});
