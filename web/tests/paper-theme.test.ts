import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { themeTokens } from "@usememos/ui";
import { describe, expect, it } from "vitest";

// Read from disk: Vitest replaces CSS imports, `?raw` included, with empty strings.
const paper = readFileSync(resolve(import.meta.dirname, "../src/themes/paper.css"), "utf8");
const defaults = readFileSync(resolve(import.meta.dirname, "../src/themes/default.css"), "utf8");

const themeColors = (stylesheet: string, theme: string) => {
  const declarations = [...stylesheet.matchAll(/([^{}]+)\{([^{}]+)\}/g)]
    .filter(([, selector]) => selector.includes(`[data-theme="${theme}"]`))
    .flatMap(([, , block]) =>
      [...block.matchAll(/(--(?:mu|memos)-[\w-]+):\s*([^;]+);/g)].map(([, name, value]) => [name, value.trim()] as const),
    );
  const raw = new Map(declarations);
  const resolveReferences = (value: string): string =>
    value.replace(/var\((--(?:mu|memos)-[\w-]+)\)/g, (_, name) => resolveReferences(raw.get(name) ?? ""));
  return new Map([...raw].map(([name, value]) => [name, resolveReferences(value)]));
};

const declarations = themeColors(paper, "paper");

const token = (name: string): string => {
  const value = declarations.get(`--mu-${name}`);
  if (!value) throw new Error(`Paper does not set --mu-${name}`);
  return value;
};

// WCAG 2 relative luminance, from sRGB or OKLCH (CSS Color 4 conversion matrices).
const linear = (channel: number): number => (channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4);

const luminanceFromRgb = (r: number, g: number, b: number): number => 0.2126 * linear(r) + 0.7152 * linear(g) + 0.0722 * linear(b);

const luminanceFromOklch = (l: number, c: number, hDegrees: number): number => {
  const h = (hDegrees * Math.PI) / 180;
  const a = c * Math.cos(h);
  const b = c * Math.sin(h);
  const l_ = (l + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m_ = (l - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s_ = (l - 0.0894841775 * a - 1.291485548 * b) ** 3;
  const r = 4.0767416621 * l_ - 3.3077115913 * m_ + 0.2309699292 * s_;
  const g = -1.2684380046 * l_ + 2.6097574011 * m_ - 0.3413193965 * s_;
  const bl = -0.0041960863 * l_ - 0.7034186147 * m_ + 1.707614701 * s_;
  // Linear sRGB already; clamp to the gamut as a browser would.
  const clamp = (v: number) => Math.min(1, Math.max(0, v));
  return 0.2126 * clamp(r) + 0.7152 * clamp(g) + 0.0722 * clamp(bl);
};

const luminance = (color: string): number => {
  const hex = color.match(/^#([0-9a-f]{3}|[0-9a-f]{6})$/i);
  if (hex) {
    const digits = hex[1];
    const n = Number.parseInt(digits.length === 3 ? [...digits].map((digit) => digit.repeat(2)).join("") : digits, 16);
    return luminanceFromRgb(((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255);
  }
  const oklch = color.match(/^oklch\(\s*([\d.]+)\s+([\d.]+)\s+([\d.]+)\s*\)$/);
  if (oklch) return luminanceFromOklch(Number(oklch[1]), Number(oklch[2]), Number(oklch[3]));
  throw new Error(`Opaque hex or oklch expected for a text pair, got ${color}`);
};

const contrastRatio = (text: string, background: string): number => {
  const [light, dark] = [luminance(text), luminance(background)].sort((a, b) => b - a);
  return (light + 0.05) / (dark + 0.05);
};

const contrast = (text: string, background: string): number => contrastRatio(token(text), token(background));

describe("Paper theme", () => {
  it("sets every theme token", () => {
    expect([...declarations.keys()].sort()).toEqual([...themeTokens].sort());
  });

  it("keeps saved selection distinct from hover and well", () => {
    expect(luminance(token("selected"))).toBeLessThan(luminance(token("hover")));
    expect(luminance(token("selected"))).toBeLessThan(luminance(token("well")));
  });

  it.each(["canvas", "surface"])("keeps control boundaries on %s at 3:1 or more", (background) => {
    expect(contrast("control-edge", background)).toBeGreaterThanOrEqual(3);
  });

  // Every text and background pair color.md's Usage column names keeps 4.5:1 or more.
  it.each([
    ["text", "canvas"],
    ["text", "surface"],
    ["text", "well"],
    ["text", "key"],
    ["text", "hover"],
    ["text", "selected"],
    ["text", "key-hover"],
    ["text-secondary", "canvas"],
    ["text-secondary", "surface"],
    ["text-secondary", "well"],
    ["text-secondary", "key"],
    ["text-secondary", "hover"],
    ["text-secondary", "key-hover"],
    ["text-disabled", "canvas"],
    ["text-disabled", "surface"],
    ["text-disabled", "well"],
    ["text-disabled", "selected"],
    ["on-primary", "primary"],
    ["on-primary", "primary-hover"],
    ["accent", "canvas"],
    ["accent", "surface"],
    ["success", "surface"],
    ["success", "success-soft"],
    ["warning", "surface"],
    ["warning", "warning-soft"],
    ["danger", "surface"],
    ["danger", "danger-soft"],
    ["on-danger", "danger-key"],
  ])("keeps %s on %s at 4.5:1 or more", (text, background) => {
    expect(contrast(text, background)).toBeGreaterThanOrEqual(4.5);
  });
});

describe.each(["light", "dark"])("%s theme", (theme) => {
  const colors = themeColors(defaults, theme);
  const cssColor = (name: string): string => {
    const value = colors.get(`--${name}`);
    if (!value) throw new Error(`${theme} does not set --${name}`);
    return value;
  };

  it.each([
    ["mu-text", "mu-canvas"],
    ["mu-text", "mu-surface"],
    ["mu-text-secondary", "mu-surface"],
    ["mu-text-secondary", "memos-sidebar"],
    ["mu-text-secondary", "mu-selected"],
    ["mu-on-primary", "mu-primary"],
    ["mu-on-primary", "mu-primary-hover"],
    ["mu-primary", "mu-selected"],
    ["mu-accent", "mu-surface"],
    ["memos-tag-text", "memos-tag-bg"],
    ["memos-tag-text", "memos-tag-hover"],
    ["memos-on-sidebar-selected", "memos-sidebar-selected"],
    ["mu-text-secondary", "memos-activity-empty"],
    ["mu-text", "memos-activity-low"],
    ["mu-text", "memos-activity-medium"],
    [theme === "dark" ? "mu-on-primary" : "mu-text", "memos-activity-high"],
  ])("keeps %s on %s readable", (text, background) => {
    expect(contrastRatio(cssColor(text), cssColor(background))).toBeGreaterThanOrEqual(4.5);
  });

  it.each(["mu-canvas", "mu-surface"])("keeps control borders visible against %s", (background) => {
    expect(contrastRatio(cssColor("mu-control-edge"), cssColor(background))).toBeGreaterThanOrEqual(3);
  });

  it("uses pure black or white text on the theme's canvas", () => {
    const expectedText = theme === "light" ? "#000000" : "#ffffff";
    expect(cssColor("mu-text")).toBe(expectedText);
    expect(cssColor("mu-text-secondary")).toBe(expectedText);
    expect(cssColor("mu-canvas")).toBe(theme === "light" ? "#fff" : "#121212");
  });

  it("uses the theme's sidebar and panel backgrounds", () => {
    expect(cssColor("memos-sidebar")).toBe(theme === "light" ? "#eaf0e8" : "#121212");
    expect(cssColor("mu-surface")).toBe(theme === "light" ? "#fefefa" : "#121212");
  });

  it("keeps the requested selected-row and composer colors", () => {
    expect(cssColor("memos-sidebar-selected")).toBe("#397354");
    expect(cssColor("memos-composer-focus")).toBe("#30cf79");
  });
});
