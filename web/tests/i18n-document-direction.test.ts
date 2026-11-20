import { afterEach, describe, expect, it, vi } from "vitest";
import i18n from "@/i18n";
import { applyDocumentLocale, applyLocaleEarly, getLocaleDirection, loadLocale, subscribeToLocaleDirection } from "@/utils/i18n";

const originalLanguage = i18n.language;
const originalLangAttribute = document.documentElement.getAttribute("lang");
const originalDirAttribute = document.documentElement.getAttribute("dir");

const restoreAttribute = (name: "lang" | "dir", value: string | null) => {
  if (value === null) {
    document.documentElement.removeAttribute(name);
  } else {
    document.documentElement.setAttribute(name, value);
  }
};

describe("document locale direction", () => {
  afterEach(async () => {
    localStorage.clear();
    restoreAttribute("lang", originalLangAttribute);
    restoreAttribute("dir", originalDirAttribute);
    await i18n.changeLanguage(originalLanguage);
  });

  it.each([
    ["zh-Hans", "ltr"],
    ["en", "ltr"],
  ] as const)("sets %s as a %s document", (locale, direction) => {
    loadLocale(locale);

    expect(document.documentElement).toHaveAttribute("lang", locale);
    expect(document.documentElement).toHaveAttribute("dir", direction);
  });

  it("uses Simplified Chinese at first load even with an English browser", () => {
    localStorage.clear();
    applyLocaleEarly();
    expect(document.documentElement).toHaveAttribute("lang", "zh-Hans");
  });

  it("notifies direction consumers before translations finish loading", () => {
    const listener = vi.fn();
    const unsubscribe = subscribeToLocaleDirection(listener);

    applyDocumentLocale("ar");

    expect(getLocaleDirection()).toBe("rtl");
    expect(listener).toHaveBeenCalledOnce();
    unsubscribe();
  });
});
