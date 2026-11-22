import { describe, expect, it } from "vitest";
import { DEFAULT_SETTING_SECTION, isSettingSectionKey, SETTINGS_SECTIONS } from "@/components/Settings/settingSections";

describe("settings sections", () => {
  it.each([
    "configuration",
    "notification",
    "webhook",
    "spaces",
    "sso",
    "member",
    "usage-limits",
  ])("treats the retired %s hash as unknown", (key) => {
    expect(isSettingSectionKey(key)).toBe(false);
  });

  it("keeps account as the default and retains the admin settings", () => {
    expect(DEFAULT_SETTING_SECTION).toBe("my-account");
    expect(SETTINGS_SECTIONS.filter((section) => section.scope === "admin").map((section) => section.key)).toEqual([
      "system",
      "security",
      "storage",
      "resource-stats",
    ]);
  });
});
