import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SCOPE_PANEL_ROW_CLASSES } from "@/components/AppSidebar/sidebar-layout";
import UserMenu from "@/components/UserMenu";
import { loadLocale } from "@/utils/i18n";
import { loadTheme } from "@/utils/theme";

const mocks = vi.hoisted(() => ({
  onOpenSettings: vi.fn(),
  logout: vi.fn(),
  updateGeneralSetting: vi.fn(),
  refetchSettings: vi.fn(),
  currentUser: { name: "users/steven", username: "steven", displayName: "Steven" } as
    | { name: string; username: string; displayName: string }
    | undefined,
}));

vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ userGeneralSetting: undefined, refetchSettings: mocks.refetchSettings, logout: mocks.logout }),
}));
vi.mock("@/hooks/useLiveMemoRefresh", () => ({ useSSEConnectionStatus: () => "connected" }));
vi.mock("@/hooks/useCurrentUser", () => ({
  default: () => mocks.currentUser,
}));
vi.mock("@/hooks/useUserQueries", () => ({
  useUpdateUserGeneralSetting: () => ({ mutate: mocks.updateGeneralSetting }),
}));
vi.mock("@/utils/i18n", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/utils/i18n")>()),
  loadLocale: vi.fn((await importOriginal<typeof import("@/utils/i18n")>()).loadLocale),
  useTranslate: () => (key: string) => key,
}));
vi.mock("@/utils/theme", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/utils/theme")>()),
  loadTheme: vi.fn((await importOriginal<typeof import("@/utils/theme")>()).loadTheme),
}));

const MenuLocation = () => {
  const { pathname, search } = useLocation();
  return <output data-testid="menu-location">{`${pathname}${search}`}</output>;
};

const renderMenu = (path = "/") =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <UserMenu onOpenSettings={mocks.onOpenSettings} />
      <MenuLocation />
    </MemoryRouter>,
  );

describe("User menu panel", () => {
  beforeEach(() => {
    mocks.currentUser = { name: "users/steven", username: "steven", displayName: "Steven" };
    localStorage.clear();
    localStorage.setItem("memos-locale", "en");
    localStorage.setItem("memos-theme", "system");
    vi.mocked(loadTheme).mockClear();
    vi.mocked(loadLocale).mockClear();
    mocks.onOpenSettings.mockReset();
    mocks.logout.mockReset();
    mocks.updateGeneralSetting.mockReset();
    mocks.refetchSettings.mockReset();
  });

  it("signs out to the sign-in page and retains account-local tag pins", async () => {
    const originalLocation = window.location;
    const replace = vi.fn();
    localStorage.setItem("memos-pinned-tags:users/steven", '["work"]');
    localStorage.setItem("temporary-account-state", "clear this");
    Object.defineProperty(window, "location", { configurable: true, value: { ...originalLocation, replace } });
    try {
      renderMenu();
      fireEvent.click(screen.getByRole("button", { name: "common.sign-out" }));
      await waitFor(() => expect(replace).toHaveBeenCalledOnce());
      expect(new URL(replace.mock.calls[0][0], originalLocation.origin).pathname).toBe("/auth");
      expect(mocks.logout).toHaveBeenCalledOnce();
      expect(localStorage.getItem("memos-pinned-tags:users/steven")).toBe('["work"]');
      expect(localStorage.getItem("temporary-account-state")).toBeNull();
    } finally {
      Object.defineProperty(window, "location", { configurable: true, value: originalLocation });
    }
  });

  it("puts the Settings action before the account", () => {
    renderMenu("/");
    const account = screen.getByRole("region", { name: "setting.account.title" });
    const settings = screen.getByRole("button", { name: "common.settings" });
    expect(within(account).getByText("Steven")).toBeInTheDocument();
    expect(settings.compareDocumentPosition(account) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(account).queryByRole("link")).not.toBeInTheDocument();
  });

  it("draws every row on one panel row grammar so icons share one column", () => {
    renderMenu("/");
    const rows = [
      screen.getByRole("button", { name: /setting.preference.theme/ }),
      screen.getByRole("button", { name: /common.language/ }),
      screen.getByRole("button", { name: "common.settings" }),
      screen.getByRole("button", { name: "common.sign-out" }),
    ];
    for (const row of rows) {
      // Every row is the package's menu row, so height, padding, gap and type line up. Color may differ (Sign out is quieter).
      expect(row).toHaveClass("mu-menu-item");
      expect(SCOPE_PANEL_ROW_CLASSES).toContain("mu-menu-item");
    }
  });

  it("opens Settings while preserving the current collection and its filters", () => {
    const collection = "/?filter=tagSearch%3Awork";
    renderMenu(collection);
    const settings = screen.getByRole("button", { name: "common.settings" });
    expect(settings).toHaveAttribute("aria-haspopup", "dialog");
    fireEvent.click(settings);
    expect(mocks.onOpenSettings).toHaveBeenCalledOnce();
    expect(screen.getByTestId("menu-location")).toHaveTextContent(collection);
  });

  it("offers guests preferences without account pages or a second sign-in", () => {
    mocks.currentUser = undefined;
    renderMenu("/404");
    expect(screen.getByRole("button", { name: /setting.preference.theme/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /common.language/ })).toBeInTheDocument();
    for (const name of ["common.inbox", "common.settings", "common.sign-out", "common.sign-in"]) {
      expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
      expect(screen.queryByRole("link", { name })).not.toBeInTheDocument();
    }
    expect(screen.queryByRole("link", { name: "common.about" })).not.toBeInTheDocument();
  });

  it("shows current preferences on rows that open their choices as submenus", async () => {
    renderMenu();
    fireEvent.click(screen.getByRole("button", { name: /setting.preference.theme.*setting.preference.theme-system/ }));
    const themes = await screen.findByRole("menu");
    expect(within(themes).getByRole("menuitemradio", { name: "setting.preference.theme-system" })).toHaveAttribute("aria-checked", "true");
    expect(
      within(themes)
        .getAllByRole("menuitemradio")
        .map((item) => item.textContent),
    ).toEqual([
      "setting.preference.theme-system",
      "setting.preference.theme-light",
      "setting.preference.theme-dark",
      "setting.preference.theme-paper",
    ]);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it.each([/setting.preference.theme/, /common.language/])("keeps the %s submenu fixed after positioning", async (row) => {
    renderMenu();
    fireEvent.click(screen.getByRole("button", { name: row }));
    const menu = await screen.findByRole("menu");
    const positioner = menu.parentElement as HTMLElement;
    await waitFor(() => {
      expect(positioner.style.opacity).not.toBe("0");
      expect(positioner.style.position).toBe("fixed");
    });
  });

  it.each([
    {
      row: /setting.preference.theme/,
      choice: "setting.preference.theme-dark",
      load: loadTheme,
      value: "default-dark",
      field: "theme",
      label: /setting.preference.theme.*setting.preference.theme-dark/,
    },
    { row: /common.language/, choice: "简体中文", load: loadLocale, value: "zh-Hans", field: "locale", label: /common.language.*简体中文/ },
  ] as const)("applies and saves $field from its submenu", async ({ row, choice, load, value, field, label }) => {
    renderMenu();
    fireEvent.click(screen.getByRole("button", { name: row }));
    fireEvent.click(await screen.findByRole("menuitemradio", { name: choice }));
    expect(load).toHaveBeenCalledWith(value);
    expect(mocks.updateGeneralSetting).toHaveBeenCalledWith(
      { generalSetting: { [field]: value }, updateMask: [field] },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
    expect(screen.getByRole("button", { name: label })).toBeInTheDocument();
  });

  it.each([
    {
      row: /setting.preference.theme/,
      choice: "setting.preference.theme-dark",
      storageKey: "memos-theme",
      value: "default-dark",
      label: /setting.preference.theme.*setting.preference.theme-dark/,
    },
    { row: /common.language/, choice: "简体中文", storageKey: "memos-locale", value: "zh-Hans", label: /common.language.*简体中文/ },
  ] as const)("persists a guest's choice of $storageKey locally without updating an account", async ({
    row,
    choice,
    storageKey,
    value,
    label,
  }) => {
    mocks.currentUser = undefined;
    const menu = renderMenu("/404");
    fireEvent.click(screen.getByRole("button", { name: row }));
    fireEvent.click(await screen.findByRole("menuitemradio", { name: choice }));
    expect(localStorage.getItem(storageKey)).toBe(value);
    expect(mocks.updateGeneralSetting).not.toHaveBeenCalled();
    expect(mocks.refetchSettings).not.toHaveBeenCalled();
    menu.unmount();
    renderMenu("/404");
    expect(screen.getByRole("button", { name: label })).toBeInTheDocument();
  });
});
