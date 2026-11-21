import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, screen, render as testingLibraryRender, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AppSidebar, { MobileAppHeader, MobileAppSidebar } from "@/components/AppSidebar";
import { User_Role } from "@/types/proto/api/user_service_pb";

const authState = vi.hoisted(() => ({
  currentUser: undefined as { name: string; username: string; displayName?: string; avatarUrl?: string; role?: User_Role } | undefined,
  memoViews: [] as Array<{ name: string; title: string }>,
}));
const sidebarState = vi.hoisted(() => ({
  mobileOpen: false,
  setMobileOpen: vi.fn(),
  setQuickFindOpen: vi.fn(),
}));
const globalEditorState = vi.hoisted(() => ({
  canOpen: true,
  openEditor: vi.fn(),
}));
const filteredStatsHook = vi.hoisted(() => vi.fn());
const tagsSectionHook = vi.hoisted(() => vi.fn());

vi.mock("@/components/MemosLogo", () => ({
  default: ({ size }: { size?: string }) => <span data-logo-size={size}>Memos logo</span>,
}));

vi.mock("@/components/MemoDisplaySettingMenu", () => ({
  default: () => <button type="button">memo.view-options</button>,
}));

vi.mock("@/components/UserMenu", () => ({
  default: () => (
    <button type="button" className="w-full">
      User menu
    </button>
  ),
}));

vi.mock("@/components/StatisticsView", () => ({
  default: () => <div>Calendar</div>,
}));

vi.mock("@/components/AppSidebar/TagsSection", () => ({
  default: (props: unknown) => {
    tagsSectionHook(props);
    return <div>Tags</div>;
  },
}));

vi.mock("@/contexts/AppSidebarContext", () => ({
  useAppSidebar: () => ({
    memoDetail: undefined,
    setMemoDetail: vi.fn(),
    mobileOpen: sidebarState.mobileOpen,
    setMobileOpen: sidebarState.setMobileOpen,
    quickFindOpen: false,
    setQuickFindOpen: sidebarState.setQuickFindOpen,
  }),
}));

vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ isInitialized: true }),
}));

vi.mock("@/contexts/GlobalMemoEditorContext", () => ({
  useGlobalMemoEditor: () => globalEditorState,
}));

vi.mock("@/contexts/InstanceContext", () => ({
  useInstance: () => ({ isInitialized: true }),
}));

vi.mock("@/contexts/MemoFilterContext", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/contexts/MemoFilterContext")>()),
  useMemoFilterContext: () => ({
    filters: [],
    memoView: undefined,
    setMemoView: vi.fn(),
  }),
}));

vi.mock("@/hooks/useCurrentUser", () => ({
  default: () => authState.currentUser,
}));

vi.mock("@/hooks/useFilteredMemoStats", () => ({
  useFilteredMemoStats: (options: unknown) => {
    filteredStatsHook(options);
    return {
      statistics: { activityStats: {}, timeBasis: "create_time" },
      tags: {},
      summary: { memoCount: 3, tagCount: 2, recordedDays: 1 },
      loading: false,
    };
  },
}));

vi.mock("@/hooks/useMediaQuery", () => ({
  default: () => true,
}));

vi.mock("@/hooks/useUserQueries", () => ({
  userKeys: {
    memoViews: (parent?: string) => ["users", "memoViews", parent],
  },
  useMemoViews: () => ({ data: authState.memoViews }),
  useUpdateUserGeneralSetting: () => ({ mutate: vi.fn() }),
}));

vi.mock("@/i18n", () => ({
  default: { language: "en" },
}));

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) => key,
}));

const render = (ui: Parameters<typeof testingLibraryRender>[0]) =>
  testingLibraryRender(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>{ui}</QueryClientProvider>,
  );

const expectExpandedNavPill = (pill: HTMLElement, label: string) => {
  expect(pill).toHaveClass("mu-s-md", "h-(--mu-row-height)", "rounded-(--mu-row-radius)", "px-1.5");
  expect(pill.firstElementChild).toHaveClass("size-4");
  const labelTrack = pill.querySelector("span.grid");
  expect(labelTrack).toHaveClass("@min-[230px]:grid-cols-[1fr]", "@min-[230px]:ps-1.5");
  expect(labelTrack).toHaveAttribute("aria-hidden", "true");
  expect(pill.querySelector("[data-sidebar-label]")).toHaveTextContent(label);
};

const expectActiveNavPill = (pill: HTMLElement, label: string) => {
  expectExpandedNavPill(pill, label);
  expect(pill).toHaveAttribute("aria-current", "page");
};

const expectDefaultNavPill = (pill: HTMLElement, label: string) => {
  expectExpandedNavPill(pill, label);
  expect(pill).not.toHaveAttribute("aria-current");
};

describe("App sidebar", () => {
  beforeEach(() => {
    authState.currentUser = { name: "users/test", username: "test" };
    authState.memoViews = [];
    sidebarState.mobileOpen = false;
    sidebarState.setMobileOpen.mockClear();
    sidebarState.setQuickFindOpen.mockClear();
    globalEditorState.canOpen = true;
    globalEditorState.openEditor.mockClear();
    filteredStatsHook.mockClear();
    tagsSectionHook.mockClear();
  });

  it("preserves the Calendar link’s date and tag filters", () => {
    const search = "?filter=tagSearch%3Awork%2CdisplayTime%3A2026-08-02";
    render(
      <MemoryRouter initialEntries={[`/${search}`]}>
        <AppSidebar />
      </MemoryRouter>,
    );
    expect(screen.getByRole("link", { name: "common.calendar" })).toHaveAttribute("href", `/${search}`);
    expect(screen.getByRole("link", { name: "common.calendar" }).querySelector(".lucide-calendar-days")).not.toBeNull();
  });

  it("shows the account trigger and opens the global memo editor", () => {
    render(
      <MemoryRouter initialEntries={["/setting"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    const account = screen.getByRole("button", { name: "common.account-menu: test" });
    const header = account.closest("[data-sidebar-header]");
    const compose = screen.getByRole("button", { name: "editor.new-memo" });
    const primaryNavigation = screen.getByRole("navigation", { name: "common.primary-navigation" });
    const search = within(primaryNavigation).getByRole("button", { name: "common.search" });

    expect(header).toHaveClass("h-13", "px-3");
    expect(account).toHaveTextContent("test");
    expect(account).toHaveClass("min-w-0", "h-9", "gap-2", "px-2");
    expect(account).not.toHaveClass("px-1");
    expect(account.firstElementChild).not.toHaveClass("flex-1");
    expect(account.querySelector(".lucide-chevrons-up-down")).not.toBeNull();
    expect(account.querySelector(".lucide-chevron-down")).toBeNull();
    expect(header).toContainElement(screen.getByRole("button", { name: /setting.preference.theme-(dark|light)/ }));
    expect(compose).toHaveClass("mu-button", "mu-button-neutral", "mu-button-icon", "mu-s-md");
    expect(compose).not.toHaveClass("rounded-full");
    expect(header).not.toContainElement(search);
    expect(search).toHaveClass("ms-auto", "h-(--mu-row-height)", "px-1.5");
    expect(search.querySelector(".lucide-search")).toHaveClass("mu-icon");

    fireEvent.click(search);
    expect(sidebarState.setQuickFindOpen).toHaveBeenCalledWith(true);
    expect(sidebarState.setMobileOpen).toHaveBeenCalledWith(false);

    fireEvent.click(compose);
    expect(globalEditorState.openEditor).toHaveBeenCalledOnce();
    expect(within(primaryNavigation).getAllByRole("link")).toHaveLength(1);
    expect(screen.queryByText("Calendar")).not.toBeInTheDocument();
  });

  it("scopes calendar statistics and tags to the signed-in account", () => {
    render(
      <MemoryRouter initialEntries={["/?creator=another-account"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(filteredStatsHook).toHaveBeenCalledWith(expect.objectContaining({ context: "collection", filter: 'creator == "users/test"' }));
    expect(tagsSectionHook).toHaveBeenCalledWith(expect.objectContaining({ scope: "users/test:home" }));
  });

  it("uses archived statistics and separate tag UI state for the same account", () => {
    render(
      <MemoryRouter initialEntries={["/archived"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(filteredStatsHook).toHaveBeenCalledWith(expect.objectContaining({ context: "archived", filter: undefined }));
    expect(tagsSectionHook).toHaveBeenCalledWith(expect.objectContaining({ scope: "users/test:archived" }));
  });

  it("opens account actions from the signed-in display name and avatar", () => {
    authState.currentUser = { name: "users/test", username: "test", displayName: "Test Account", avatarUrl: "/avatar.png" };
    render(
      <MemoryRouter initialEntries={["/archived"]}>
        <AppSidebar />
      </MemoryRouter>,
    );
    expect(screen.queryByRole("contentinfo")).not.toBeInTheDocument();
    const menu = screen.getByRole("button", { name: "common.account-menu: Test Account" });
    expect(menu).toHaveTextContent("Test Account");
    expect(menu).toHaveAttribute("title", "Test Account");
    expect(menu.querySelector("img")).toHaveAttribute("src", "/avatar.png");
    fireEvent.click(menu);
    expect(screen.getByRole("button", { name: "User menu" })).toBeInTheDocument();
  });

  it("hides Compose when the composer reports it is not available", () => {
    globalEditorState.canOpen = false;
    render(
      <MemoryRouter initialEntries={["/"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(screen.queryByRole("button", { name: "editor.new-memo" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "common.search" })).toBeInTheDocument();
  });

  it("shows the compact visitor navigation and sign-in link on an error page", () => {
    authState.currentUser = undefined;
    globalEditorState.canOpen = false;
    render(
      <MemoryRouter initialEntries={["/404"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    const brand = screen.getByRole("button", { name: "common.account-menu: common.memos" });
    expect(within(brand).getByText("Memos logo")).toHaveAttribute("data-logo-size", "header");
    fireEvent.click(brand);
    expect(screen.getByRole("button", { name: "User menu" })).toBeInTheDocument();
    const primaryNavigation = screen.getByRole("navigation", { name: "common.primary-navigation" });
    expect(primaryNavigation).toHaveClass("h-(--mu-row-height)", "items-center", "gap-0.5", "px-3");
    expect(primaryNavigation).not.toHaveClass("flex-col");
    const navigation = within(primaryNavigation);
    expect(navigation.getByRole("button", { name: "common.search" })).toHaveClass("ms-auto", "h-(--mu-row-height)", "px-1.5");
    const home = navigation.getByRole("link", { name: "common.calendar" });
    expectDefaultNavPill(home, "common.calendar");
    expect(home).toHaveAttribute("href", "/");
    expect(navigation.getAllByRole("link")).toHaveLength(1);
    expect(navigation.queryByRole("link", { name: "common.about" })).not.toBeInTheDocument();
    expect(navigation.queryByRole("link", { name: "common.attachments" })).not.toBeInTheDocument();
    const signIn = screen.getByRole("link", { name: "common.sign-in-to-memos" });
    expect(signIn).toHaveClass("w-full", "px-5");
    expect(signIn).not.toHaveClass("rounded-mu-md");
    expect(signIn.closest("footer")).not.toBeNull();
    expect(screen.queryByRole("link", { name: "trash.title" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "editor.new-memo" })).not.toBeInTheDocument();
  });

  it.each(["/403", "/404", "/unknown", "/inbox", "/spaces/product"])("does not render collection controls on %s", (path) => {
    render(
      <MemoryRouter initialEntries={[path]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(screen.queryByRole("link", { name: "common.about" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "common.resources" })).not.toBeInTheDocument();
    expect(screen.queryByText("Calendar")).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "common.statistics" })).not.toBeInTheDocument();
    expect(screen.queryByText("common.views")).not.toBeInTheDocument();
    expect(screen.queryByText("Tags")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "common.account-menu: test" })).toBeInTheDocument();
    const navigation = within(screen.getByRole("navigation", { name: "common.primary-navigation" }));
    expectDefaultNavPill(navigation.getByRole("link", { name: "common.calendar" }), "common.calendar");
    expect(navigation.getAllByRole("link")).toHaveLength(1);
    expect(screen.queryByRole("link", { name: "common.inbox" })).not.toBeInTheDocument();
    expect(screen.queryByRole("contentinfo")).not.toBeInTheDocument();
  });

  it("places statistics above the calendar and views below it, with a separate action for display options", () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    const sidebar = screen.getByRole("complementary");
    const calendar = screen.getByText("Calendar");
    const views = screen.getByText("common.views");
    const tags = screen.getByText("Tags");
    const statistics = screen.getByRole("region", { name: "common.statistics" });
    const trash = screen.getByRole("link", { name: "trash.title" });
    expect(sidebar).toHaveClass("overflow-y-auto", "overscroll-y-contain");
    expect(sidebar).toContainElement(trash);
    expect(tags.compareDocumentPosition(trash) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(trash.parentElement).not.toHaveClass("border-t");
    expect(statistics.parentElement).toHaveClass("gap-6");
    expect(views.closest("[data-sidebar-collection-list]")).toContainElement(tags);
    expect(calendar.closest("[data-sidebar-collection-list]")).toBeNull();
    const counts = within(statistics).getAllByRole("definition");
    expect(counts.map((count) => count.textContent)).toEqual(["3", "2", "1"]);
    expect(statistics.compareDocumentPosition(calendar) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getByRole("heading", { name: "common.views", level: 2 })).toBeInTheDocument();
    expect(calendar.compareDocumentPosition(views) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    const viewOptions = screen.getByRole("button", { name: "memo.view-options" });
    const createView = screen.getByRole("button", { name: "setting.memo-view.create" });
    expect(viewOptions.closest("nav")).toBe(screen.getByRole("navigation", { name: "common.primary-navigation" }));
    expect(screen.getByRole("link", { name: "common.calendar" }).nextElementSibling).toBe(viewOptions);
    expect(viewOptions.parentElement).toHaveClass("bg-mu-hover", "rounded-mu-md");
    expect(viewOptions.closest("a")).toBeNull();
    expect(createView).toHaveTextContent("setting.memo-view.create");
    expect(screen.queryByRole("button", { name: "common.tasks" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "common.explore" })).not.toBeInTheDocument();

    const home = screen.getByRole("link", { name: "common.calendar" });
    expectActiveNavPill(home, "common.calendar");
    expect(home.querySelector(".lucide-chevron-down")).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "common.explore" })).not.toBeInTheDocument();
  });

  it("keeps Calendar display settings visible when a date filter is active", () => {
    render(
      <MemoryRouter initialEntries={["/?filter=displayTime%3A2026-08-02"]}>
        <AppSidebar />
      </MemoryRouter>,
    );
    expect(
      within(screen.getByRole("navigation", { name: "common.primary-navigation" })).getByRole("button", { name: "memo.view-options" }),
    ).toBeInTheDocument();
  });

  it.each(["/setting", "/views", "/archived", "/trash"])("hides Calendar display settings on other views: %s", (path) => {
    render(
      <MemoryRouter initialEntries={[path]}>
        <AppSidebar />
      </MemoryRouter>,
    );
    expect(screen.queryByRole("button", { name: "memo.view-options" })).not.toBeInTheDocument();
  });

  it("opens the create form from the empty Views section and closes the mobile sidebar", () => {
    const LocationProbe = () => {
      const location = useLocation();
      return <output data-testid="view-destination">{JSON.stringify({ pathname: location.pathname, state: location.state })}</output>;
    };
    render(
      <MemoryRouter initialEntries={["/"]}>
        <AppSidebar />
        <LocationProbe />
      </MemoryRouter>,
    );
    expect(screen.getAllByRole("button", { name: "setting.memo-view.create" })).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "setting.memo-view.create" }));
    expect(screen.getByTestId("view-destination")).toHaveTextContent(JSON.stringify({ pathname: "/views", state: { openCreate: true } }));
    expect(sidebarState.setMobileOpen).toHaveBeenCalledWith(false);
  });

  it("uses fixed-density text-only actions for a saved view", async () => {
    authState.memoViews = [{ name: "memoViews/1", title: "testgp" }];
    render(
      <MemoryRouter initialEntries={["/"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "common.edit testgp" }));

    const menu = await screen.findByRole("menu");
    expect(menu).toHaveClass("mu-s-sm");
    const editItem = screen.getByRole("menuitem", { name: "common.edit" });
    const deleteItem = screen.getByRole("menuitem", { name: "common.delete" });
    expect(editItem.querySelector("svg")).toBeNull();
    expect(deleteItem.querySelector("svg")).toBeNull();
    expect(deleteItem).toHaveClass("mu-menu-item-danger");
  });

  it("lists only the available Settings sections for an administrator", () => {
    authState.currentUser = { name: "users/test", username: "test", role: User_Role.ADMIN };
    render(
      <MemoryRouter initialEntries={["/setting"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: "common.basic", level: 2 })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "common.admin", level: 2 })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "setting.my-account.label" })).toHaveAttribute("href", "/setting#my-account");
    expect(screen.getByRole("link", { name: "setting.system.label" })).toHaveAttribute("href", "/setting#system");
    const destinations = screen.getAllByRole("link").map((link) => link.getAttribute("href"));
    for (const retired of ["configuration", "notification", "webhook", "spaces", "sso"]) {
      expect(destinations).not.toContain(`/setting#${retired}`);
    }
  });

  it.each(["/trash", "/archived"])("links directly to the personal feed from %s", (path) => {
    render(
      <MemoryRouter initialEntries={[path]}>
        <AppSidebar />
      </MemoryRouter>,
    );
    const home = screen.getByRole("link", { name: "common.calendar" });
    expectDefaultNavPill(home, "common.calendar");
    expect(home).toHaveAttribute("href", "/");
    expect(screen.queryByRole("button", { name: "common.explore" })).not.toBeInTheDocument();
  });

  it("keeps the mobile header limited to navigation, account and theme controls", () => {
    render(
      <MemoryRouter initialEntries={["/about"]}>
        <MobileAppHeader />
      </MemoryRouter>,
    );

    expect(screen.getByRole("button", { name: "common.open-navigation" })).toHaveAttribute("data-mobile-navigation-trigger");
    const account = screen.getByRole("button", { name: "common.account-menu: test" });
    expect(account).toHaveClass("h-9", "gap-1.5", "px-1");
    expect(screen.getByRole("button", { name: /setting.preference.theme-(dark|light)/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "common.search" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "editor.new-memo" })).not.toBeInTheDocument();
  });

  it("exposes Compose in the mobile navigation drawer", () => {
    sidebarState.mobileOpen = true;
    render(
      <MemoryRouter initialEntries={["/"]}>
        <MobileAppSidebar />
      </MemoryRouter>,
    );

    expect(screen.getByRole("button", { name: "common.search" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /setting.preference.theme-(dark|light)/ })).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "common.statistics" }).parentElement).toHaveClass("gap-3");
    expect(screen.getByText("Tags").closest("[data-sidebar-collection-list]")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "editor.new-memo" }));
    expect(globalEditorState.openEditor).toHaveBeenCalledOnce();
  });

  it("keeps an accessible Close control in the mobile navigation drawer", () => {
    sidebarState.mobileOpen = true;
    render(
      <MemoryRouter initialEntries={["/about"]}>
        <MobileAppSidebar />
      </MemoryRouter>,
    );

    const close = screen.getByRole("button", { name: "common.close" });
    expect(screen.getByRole("dialog")).toContainElement(close);
    expect(close).toHaveClass("mu-visually-hidden");
    fireEvent.click(close);
    expect(sidebarState.setMobileOpen.mock.calls.at(-1)?.[0]).toBe(false);
  });
});
