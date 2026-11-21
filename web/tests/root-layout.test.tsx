import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useAppSidebar } from "@/contexts/AppSidebarContext";
import { TimelineViewProvider } from "@/contexts/ViewContext";
import RootLayout from "@/layouts/RootLayout";

const authState = vi.hoisted(() => ({ currentUser: { name: "users/test" } as { name: string } | undefined }));
const instanceState = vi.hoisted(() => ({ accessMode: 2, instanceUrl: "https://example.com", demo: true }));

vi.mock("@/components/AppSidebar", () => ({
  default: () => {
    const { openSettings } = useAppSidebar();
    return (
      <aside data-testid="desktop-sidebar">
        <button type="button" onClick={(event) => openSettings(event.currentTarget)}>
          Open settings
        </button>
      </aside>
    );
  },
  MobileAppHeader: () => <header data-testid="mobile-header">Mobile header</header>,
  MobileAppSidebar: () => <aside data-testid="mobile-sidebar">Mobile sidebar</aside>,
  QuickFindDialog: () => <div data-testid="quick-find">Quick Find</div>,
  SIDEBAR_WIDTH_VAR: "--app-sidebar-width",
  SidebarResizeHandle: () => <div data-testid="resize-handle" />,
  useSidebarWidth: () => ({ width: 256, minWidth: 192, maxWidth: 384, setWidth: vi.fn() }),
}));

vi.mock("@/components/Settings/SettingsContent", () => ({
  default: () => <div>Active settings section</div>,
}));

vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ currentUser: authState.currentUser, isInitialized: true }),
}));

vi.mock("@/contexts/InstanceContext", () => ({
  useInstance: () => ({ profile: instanceState, isInitialized: true }),
}));

vi.mock("@/hooks/useCurrentUser", () => ({
  default: () => authState.currentUser,
}));

vi.mock("@/hooks/useMediaQuery", () => ({
  default: () => true,
}));

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) => key,
}));

const SHELL_TEST_IDS = ["desktop-sidebar", "mobile-sidebar", "mobile-header", "quick-find"];

const RouteState = () => {
  const navigate = useNavigate();
  const location = useLocation();
  return (
    <div>
      <output data-testid="route">
        {location.pathname}
        {location.search}
      </output>
      <button type="button" onClick={() => navigate("/inbox")}>
        Open inbox
      </button>
    </div>
  );
};

describe("RootLayout shell", () => {
  beforeEach(() => {
    authState.currentUser = { name: "users/test" };
    instanceState.accessMode = 2;
    localStorage.removeItem("memos-view-setting");
  });

  it("keeps navigation and search available across routes", () => {
    render(
      <MemoryRouter initialEntries={["/setting"]}>
        <TimelineViewProvider>
          <Routes>
            <Route element={<RootLayout />}>
              <Route path="setting" element={<RouteState />} />
              <Route path="inbox" element={<RouteState />} />
            </Route>
          </Routes>
        </TimelineViewProvider>
      </MemoryRouter>,
    );

    for (const testId of SHELL_TEST_IDS) {
      expect(screen.getByTestId(testId)).toBeInTheDocument();
    }
    fireEvent.click(screen.getByRole("button", { name: "Open inbox" }));

    expect(screen.getByTestId("route")).toHaveTextContent("/inbox");
    for (const testId of SHELL_TEST_IDS) {
      expect(screen.getByTestId(testId)).toBeInTheDocument();
    }
  });

  it("opens and closes Settings while keeping the active feed and its filter", async () => {
    render(
      <MemoryRouter initialEntries={["/archived?filter=tagSearch%3Awork"]}>
        <TimelineViewProvider>
          <Routes>
            <Route element={<RootLayout />}>
              <Route path="archived" element={<RouteState />} />
            </Route>
          </Routes>
        </TimelineViewProvider>
      </MemoryRouter>,
    );

    const route = screen.getByTestId("route");
    const open = screen.getByRole("button", { name: "Open settings" });
    fireEvent.click(open);
    const dialog = await screen.findByRole("dialog", { name: "common.settings" });
    expect(await within(dialog).findByText("Active settings section")).toBeVisible();
    expect(screen.getByTestId("route")).toBe(route);
    expect(route).toHaveTextContent("/archived?filter=tagSearch%3Awork");
    fireEvent.click(within(dialog).getByRole("button", { name: "common.close" }));
    await waitFor(() => expect(dialog).not.toBeInTheDocument());
    expect(route).toBeInTheDocument();
    expect(open).toHaveFocus();
  });

  it("gates anonymous visitors by access mode even when an instance URL is configured", () => {
    authState.currentUser = undefined;
    instanceState.accessMode = 1;

    render(
      <MemoryRouter initialEntries={["/explore"]}>
        <TimelineViewProvider>
          <Routes>
            <Route element={<RootLayout />}>
              <Route path="explore" element={<div>Explore</div>} />
            </Route>
            <Route path="auth" element={<RouteState />} />
          </Routes>
        </TimelineViewProvider>
      </MemoryRouter>,
    );

    expect(screen.getByTestId("route")).toHaveTextContent("/auth");
  });

  it.each([
    { path: "/", preferences: {}, width: "max-w-5xl" },
    { path: "/", preferences: { maxColumns: 3 }, width: "max-w-7xl" },
    { path: "/", preferences: { fullWidth: true }, width: undefined },
    { path: "/", preferences: { maxColumns: 0 }, width: undefined },
  ])("sizes the centered shell for $path with $preferences", ({ path, preferences, width }) => {
    localStorage.setItem("memos-view-setting", JSON.stringify(preferences));

    render(
      <MemoryRouter initialEntries={[path]}>
        <TimelineViewProvider>
          <Routes>
            <Route element={<RootLayout />}>
              <Route index element={<RouteState />} />
            </Route>
          </Routes>
        </TimelineViewProvider>
      </MemoryRouter>,
    );

    const shell = screen.getByRole("main").parentElement;
    expect(shell).toHaveClass("mx-auto");
    if (width) expect(shell).toHaveClass(width);
    else expect(shell).not.toHaveClass(/max-w-[57]xl/);
    expect(screen.getByTestId("desktop-sidebar").parentElement).toHaveClass("sticky");
  });
});
