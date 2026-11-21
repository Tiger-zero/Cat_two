import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ThemeToggle from "@/components/AppSidebar/ThemeToggle";
import { loadTheme } from "@/utils/theme";

const mocks = vi.hoisted(() => ({
  currentUser: undefined as { name: string } | undefined,
  isUserSettingsInitialized: true,
  mutate: vi.fn(),
  refetchSettings: vi.fn(),
}));

vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => mocks }));
vi.mock("@/hooks/useUserQueries", () => ({ useUpdateUserGeneralSetting: () => ({ mutate: mocks.mutate, isPending: false }) }));
vi.mock("@/utils/i18n", async (original) => ({
  ...(await original<typeof import("@/utils/i18n")>()),
  useTranslate: () => (key: string) => key,
}));

const darkLabel = "setting.preference.theme-dark";
const lightLabel = "setting.preference.theme-light";

describe("quick theme toggle", () => {
  beforeEach(() => {
    mocks.currentUser = undefined;
    mocks.isUserSettingsInitialized = true;
    mocks.mutate.mockReset();
    mocks.refetchSettings.mockReset();
    loadTheme("default");
  });

  it("applies and stores both guest choices and keeps mounted controls in sync", () => {
    render(
      <>
        <ThemeToggle />
        <ThemeToggle />
      </>,
    );

    fireEvent.click(screen.getAllByRole("button", { name: darkLabel })[0]);
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
    expect(localStorage.getItem("memos-theme")).toBe("default-dark");
    expect(screen.getAllByRole("button", { name: lightLabel })).toHaveLength(2);

    fireEvent.click(screen.getAllByRole("button", { name: lightLabel })[1]);
    expect(document.documentElement).toHaveAttribute("data-theme", "light");
    expect(localStorage.getItem("memos-theme")).toBe("default");
    expect(mocks.mutate).not.toHaveBeenCalled();
  });

  it("follows an external theme selection and saves the signed-in preference", () => {
    mocks.currentUser = { name: "users/alice" };
    render(<ThemeToggle />);
    act(() => loadTheme("default-dark"));

    fireEvent.click(screen.getByRole("button", { name: lightLabel }));
    expect(document.documentElement).toHaveAttribute("data-theme", "light");
    expect(mocks.mutate).toHaveBeenCalledWith(
      { generalSetting: { theme: "default" }, updateMask: ["theme"] },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
    mocks.mutate.mock.calls[0][1].onSuccess();
    expect(mocks.refetchSettings).toHaveBeenCalledOnce();
  });

  it("switches in the current session when storage is unavailable", () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("storage disabled");
    });
    try {
      render(<ThemeToggle />);
      fireEvent.click(screen.getByRole("button", { name: darkLabel }));
      expect(document.documentElement).toHaveAttribute("data-theme", "dark");
      expect(screen.getByRole("button", { name: lightLabel })).toBeInTheDocument();
    } finally {
      setItem.mockRestore();
      act(() => loadTheme("default"));
    }
  });
});
