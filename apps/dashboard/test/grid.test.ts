import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

/**
 * The gutter is the spine of the main column, so it is asserted against the
 * stylesheet rather than the DOM: jsdom never loads the CSS, and the failure
 * this guards against is a component quietly hardcoding its own gutter again.
 * Four of them once did, and primary text began at four different left edges.
 */
const css = readFileSync(fileURLToPath(new URL("../src/style.css", import.meta.url)), "utf8");

const ruleFor = (selector: string): string => {
  const start = css.indexOf(`\n${selector} {`) >= 0 ? css.indexOf(`\n${selector} {`) : css.indexOf(`\n${selector} `);
  expect(start, `no rule found for ${selector}`).toBeGreaterThan(-1);
  return css.slice(start, css.indexOf("}", start));
};

describe("the main column's row grid", () => {
  it("defines the gutter once", () => {
    expect(css).toContain("--row-icon: 22px;");
    expect(css).toContain("--row-gap: 12px;");
  });

  for (const selector of [".converge", ".mini", ".session-row", ".person-row"]) {
    it(`draws ${selector} on the shared gutter`, () => {
      const rule = ruleFor(selector);
      expect(rule).toContain("var(--row-icon)");
      expect(rule).toContain("var(--row-gap)");
      // A literal pixel gutter is the regression this file exists to catch.
      expect(/grid-template-columns:\s*\d+px/.test(rule)).toBe(false);
    });
  }
});
