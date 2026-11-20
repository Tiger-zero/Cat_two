import { afterEach, describe, expect, it } from "vitest";
import { locales } from "@/i18n";
import { getLocaleWithFallback, loadLocale, localeMatchesSearch, normalizeLocaleSearchText } from "@/utils/i18n";

describe("locale selection", () => {
  afterEach(() => {
    localStorage.clear();
  });

  it("offers only Simplified Chinese and English, in that order", () => {
    expect(locales).toEqual(["zh-Hans", "en"]);
    expect(localeMatchesSearch("zh-Hans", "中文", "en")).toBe(true);
    expect(localeMatchesSearch("en", "英语", "zh-Hans")).toBe(true);
    expect(localeMatchesSearch("en", "romanian", "en")).toBe(false);
  });

  it("normalizes case and diacritics for locale search", () => {
    expect(normalizeLocaleSearchText("Français")).toBe("francais");
  });

  it("defaults to Simplified Chinese and handles older account preferences", () => {
    expect(getLocaleWithFallback()).toBe("zh-Hans");
    expect(getLocaleWithFallback("ja")).toBe("zh-Hans");
    expect(getLocaleWithFallback("zh-Hant")).toBe("zh-Hans");
    expect(getLocaleWithFallback("en-GB")).toBe("en");
  });

  it("persists English when chosen and gives the account preference priority", () => {
    expect(loadLocale("en")).toBe("en");
    expect(getLocaleWithFallback()).toBe("en");
    expect(getLocaleWithFallback("zh-Hans")).toBe("zh-Hans");
  });
});
