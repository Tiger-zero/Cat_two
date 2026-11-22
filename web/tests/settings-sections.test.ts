import { describe, expect, it } from "vitest";
import {
  DEFAULT_SETTING_SECTION,
  isSettingSectionKey,
  resolveSettingSectionKey,
  SETTINGS_SECTIONS,
} from "@/components/Settings/settingSections";

describe("settings sections", () => {
  it.each(["configuration", "webhook", "spaces", "sso", "member", "usage-limits"])("treats the retired %s hash as unknown", (key) => {
    expect(isSettingSectionKey(key)).toBe(false);
    expect(resolveSettingSectionKey(key)).toBe("my-account");
  });

  it("keeps account as the default and retains the admin settings", () => {
    expect(DEFAULT_SETTING_SECTION).toBe("my-account");
    expect(SETTINGS_SECTIONS.filter((section) => section.scope === "admin").map((section) => section.key)).toEqual([
      "system",
      "security",
      "notification",
      "network",
      "storage",
    ]);
  });

  it("opens storage for old Resources links without keeping a second sidebar entry", () => {
    expect(isSettingSectionKey("resource-stats")).toBe(false);
    expect(resolveSettingSectionKey("resource-stats")).toBe("storage");
    expect(resolveSettingSectionKey("storage")).toBe("storage");
  });
});
