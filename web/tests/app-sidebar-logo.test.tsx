import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, screen, render as testingLibraryRender, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AppSidebar, { MobileAppHeader, MobileAppSidebar } from "@/components/AppSidebar";
import { type MemoFilter } from "@/contexts/MemoFilterContext";
import { getCollectionCreator, resolveCollectionRoute } from "@/router/routes";

const authState = vi.hoisted(() => ({
  currentUser: { name: "users/test" } as { name: string; username?: string } | undefined,
  memoViews: [] as Array<{ name: string; title: string }>,
  notifications: [] as Array<{ status: number }>,
  guestCreator: undefined as { username: string; displayName: string; avatarUrl: string } | undefined,
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
const spaceState = vi.hoisted(() => ({
  spaces: [] as Array<{ name: string; title: string; description: string }>,
  selectedSpace: undefined as { name: string; title: string; description: string } | undefined,
  selectSpace: vi.fn(),
}));
const filteredStatsHook = vi.hoisted(() => vi.fn());
const filterState = vi.hoisted(() => ({ filters: [] as MemoFilter[] }));
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

vi.mock("@/components/CreateSpaceDialog", () => ({
  default: () => null,
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
    inboxFilter: "all",
    setInboxFilter: vi.fn(),
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
    filters: filterState.filters,
    memoView: undefined,
    setMemoView: vi.fn(),
  }),
}));

vi.mock("@/contexts/SpaceContext", () => ({
  useSpaceContext: () => {
    const location = useLocation();
    const { spaceName, isCollection } = resolveCollectionRoute(location.pathname);
    const duplicateSpaceTitles = new Set(
      spaceState.spaces
        .filter((space, index) => spaceState.spaces.findIndex((candidate) => candidate.title === space.title) !== index)
        .map((space) => space.title),
    );
    return {
      ...spaceState,
      selectedSpaceName: spaceName,
      creatorUsername: isCollection ? getCollectionCreator(location.search) : undefined,
      selectedSpace: spaceName ? spaceState.selectedSpace : undefined,
      memoFilter: spaceName ? `space == ${JSON.stringify(spaceName)}` : undefined,
      duplicateSpaceTitles,
      isLoadingSpaces: false,
      isSpacesError: false,
    };
  },
}));

vi.mock("@/hooks/useCurrentUser", () => ({
  default: () => authState.currentUser,
}));

vi.mock("@/hooks/useFilteredMemoStats", () => ({
  useFilteredMemoStats: (options: unknown) => {
    filteredStatsHook(options);
    return { statistics: { activityStats: {}, timeBasis: "create_time" }, tags: {} };
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
  useNotifications: () => ({ data: authState.notifications }),
  useUser: () => ({ data: authState.guestCreator }),
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

describe("App sidebar logo", () => {
  beforeEach(() => {
    authState.currentUser = { name: "users/test" };
    authState.memoViews = [];
    authState.notifications = [];
    authState.guestCreator = undefined;
    sidebarState.mobileOpen = false;
    sidebarState.setMobileOpen.mockClear();
    sidebarState.setQuickFindOpen.mockClear();
    globalEditorState.canOpen = true;
    globalEditorState.openEditor.mockClear();
    spaceState.spaces = [];
    spaceState.selectedSpace = undefined;
    spaceState.selectSpace.mockClear();
    filteredStatsHook.mockClear();
    filterState.filters = [];
    tagsSectionHook.mockClear();
  });

  it("preserves the Calendar link’s Space, creator and filters", () => {
    render(
      <MemoryRouter initialEntries={["/spaces/product?creator=alice&filter=tagSearch%3Awork"]}>
        <AppSidebar />
      </MemoryRouter>,
    );
    const search = "?filter=tagSearch%3Awork&creator=alice";
    expect(screen.getByRole("link", { name: "common.calendar" })).toHaveAttribute("href", `/spaces/product${search}`);
    expect(screen.getByRole("link", { name: "common.calendar" }).querySelector(".lucide-calendar-days")).not.toBeNull();
  });

  it("shows the context switcher and opens the global memo editor", () => {
    render(
      <MemoryRouter initialEntries={["/setting"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    const switcher = screen.getByRole("button", { name: "space.switch: common.memos" });
    const header = switcher.closest("[data-sidebar-header]");
    const compose = screen.getByRole("button", { name: "editor.new-memo" });
    const primaryNavigation = screen.getByRole("navigation", { name: "common.primary-navigation" });
    const search = within(primaryNavigation).getByRole("button", { name: "common.search" });

    expect(header).toHaveClass("h-13", "px-3");
    expect(switcher).toHaveTextContent("Memos logo");
    expect(switcher).toHaveClass("min-w-0", "h-9", "gap-2", "px-2");
    expect(switcher).not.toHaveClass("px-1");
    expect(switcher.firstElementChild).not.toHaveClass("flex-1");
    expect(within(switcher).getByText("Memos logo")).toHaveAttribute("data-logo-size", "header");
    expect(switcher.querySelector(".lucide-chevrons-up-down")).not.toBeNull();
    expect(switcher.querySelector(".lucide-chevron-down")).toBeNull();
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

  it.each([
    "/spaces/product",
    "/spaces/product/explore",
    "/spaces/product/Explore/",
  ])("shows the selected Space only on collection route %s", (path) => {
    const product = { name: "spaces/product", title: "Product", description: "" };
    spaceState.spaces = [product];
    spaceState.selectedSpace = product;

    render(
      <MemoryRouter initialEntries={[path]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(screen.getByRole("button", { name: "space.switch: Product" })).toBeInTheDocument();
  });

  it.each([
    "/archived",
    "/inbox",
    "/setting",
    "/about",
    "/views",
    "/memos/123",
    "/memos/shares/token",
    "/404",
  ])("shows Memos in the switcher on global page %s", (path) => {
    const product = { name: "spaces/product", title: "Product", description: "" };
    spaceState.spaces = [product];
    spaceState.selectedSpace = product;

    render(
      <MemoryRouter initialEntries={[path]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    const brand = screen.getByRole("button", { name: "space.switch: common.memos" });
    expect(brand).toHaveClass("h-9", "gap-2", "px-2");
    expect(within(brand).getByText("Memos logo")).toHaveAttribute("data-logo-size", "header");
  });

  it("scopes collection statistics to the selected Space", () => {
    render(
      <MemoryRouter initialEntries={["/spaces/product/explore"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(filteredStatsHook).toHaveBeenCalledWith(expect.objectContaining({ context: "collection", filter: 'space == "spaces/product"' }));
  });

  it("includes authorized Space memos in All Explore statistics", () => {
    render(
      <MemoryRouter initialEntries={["/explore"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(filteredStatsHook).toHaveBeenCalledWith(expect.objectContaining({ context: "collection", filter: undefined }));
  });

  it("keeps Archived statistics and tag UI state unscoped by Space", () => {
    render(
      <MemoryRouter initialEntries={["/archived"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(filteredStatsHook).toHaveBeenCalledWith(expect.objectContaining({ context: "archived", filter: undefined }));
    expect(tagsSectionHook).toHaveBeenCalledWith(expect.objectContaining({ scope: "archived" }));
  });

  it("opens account actions from the username across collections", () => {
    authState.currentUser = { name: "users/test", username: "test" };
    const product = { name: "spaces/product", title: "Product", description: "" };
    spaceState.spaces = [product];
    spaceState.selectedSpace = product;
    render(
      <MemoryRouter initialEntries={["/spaces/product"]}>
        <AppSidebar />
      </MemoryRouter>,
    );
    expect(screen.queryByRole("contentinfo")).not.toBeInTheDocument();
    const menu = screen.getByRole("button", { name: "common.account-menu: test" });
    expect(menu).toHaveTextContent("test");
    expect(menu).toHaveAttribute("title", "test · Product");
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

  it("shows the compact public navigation for a guest", () => {
    authState.currentUser = undefined;
    globalEditorState.canOpen = false;
    render(
      <MemoryRouter initialEntries={["/explore"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(screen.getByRole("button", { name: "space.switch: common.memos" })).toHaveTextContent("Memos logo");
    const primaryNavigation = screen.getByRole("navigation", { name: "common.primary-navigation" });
    expect(primaryNavigation).toHaveClass("h-(--mu-row-height)", "items-center", "gap-0.5", "px-3");
    expect(primaryNavigation).not.toHaveClass("flex-col");
    const navigation = within(primaryNavigation);
    expect(navigation.getByRole("button", { name: "common.search" })).toHaveClass("ms-auto", "h-(--mu-row-height)", "px-1.5");
    expectActiveNavPill(navigation.getByRole("link", { name: "common.calendar" }), "common.calendar");
    expect(navigation.getAllByRole("link")).toHaveLength(1);
    expect(navigation.queryByRole("link", { name: "common.about" })).not.toBeInTheDocument();
    expect(navigation.queryByRole("link", { name: "common.attachments" })).not.toBeInTheDocument();
    const signIn = screen.getByRole("link", { name: "common.sign-in-to-memos" });
    expect(signIn).toHaveClass("w-full", "px-5");
    expect(signIn).not.toHaveClass("rounded-mu-md");
    expect(signIn.closest("footer")).not.toBeNull();
    expect(screen.queryByRole("button", { name: "editor.new-memo" })).not.toBeInTheDocument();
  });

  it("shows the selected creator in the guest sidebar and mobile header", () => {
    authState.currentUser = undefined;
    authState.guestCreator = { username: "steven", displayName: "Steven", avatarUrl: "/avatar.png" };

    render(
      <MemoryRouter initialEntries={["/?creator=steven"]}>
        <AppSidebar />
        <MobileAppHeader />
      </MemoryRouter>,
    );

    const creatorLinks = screen.getAllByRole("button", { name: "space.switch: Steven" });
    expect(creatorLinks).toHaveLength(2);
    for (const link of creatorLinks) {
      expect(link.querySelector("img")).toHaveAttribute("src", "/avatar.png");
    }
    expect(screen.queryByRole("link", { name: "Memos logo" })).not.toBeInTheDocument();
  });

  it("shows the selected username while the guest profile is loading", () => {
    authState.currentUser = undefined;

    render(
      <MemoryRouter initialEntries={["/?creator=steven"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(screen.getByRole("button", { name: "space.switch: steven" })).toHaveTextContent("steven");
  });

  it.each(["/403", "/404", "/unknown"])("uses the common sidebar without inheriting collection content on %s", (path) => {
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
    expect(screen.getByRole("button", { name: "space.switch: common.memos" })).toBeInTheDocument();
    const navigation = within(screen.getByRole("navigation", { name: "common.primary-navigation" }));
    expectDefaultNavPill(navigation.getByRole("link", { name: "common.calendar" }), "common.calendar");
    expect(navigation.getAllByRole("link")).toHaveLength(1);
    expect(screen.queryByRole("link", { name: "common.inbox" })).not.toBeInTheDocument();
    expect(screen.queryByRole("contentinfo")).not.toBeInTheDocument();
  });

  it("uses a visitor sidebar for a guest on a route without contextual content", () => {
    authState.currentUser = undefined;
    render(
      <MemoryRouter initialEntries={["/404"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    const navigation = within(screen.getByRole("navigation", { name: "common.primary-navigation" }));
    const home = navigation.getByRole("link", { name: "common.calendar" });
    expect(home).toHaveAttribute("href", "/explore");
    expectDefaultNavPill(home, "common.calendar");
    expect(screen.queryByRole("link", { name: "common.attachments" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "common.inbox" })).not.toBeInTheDocument();
    expect(navigation.queryByRole("link", { name: "common.about" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "common.sign-in-to-memos" }).closest("footer")).not.toBeNull();
  });

  it("uses a compact scope menu and places views below the calendar", async () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    const calendar = screen.getByText("Calendar");
    const views = screen.getByText("common.views");
    expect(screen.getByRole("region", { name: "common.statistics" })).toBeInTheDocument();
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

  it.each([
    "/explore",
    "/?creator=alice",
    "/spaces/test",
    "/spaces/test/explore",
  ])("shows Calendar display settings in each Calendar scope: %s", (path) => {
    render(
      <MemoryRouter initialEntries={[path]}>
        <AppSidebar />
      </MemoryRouter>,
    );
    expect(
      within(screen.getByRole("navigation", { name: "common.primary-navigation" })).getByRole("button", { name: "memo.view-options" }),
    ).toBeInTheDocument();
  });

  it.each(["/setting", "/inbox", "/archived"])("hides Calendar display settings on other views: %s", (path) => {
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

  it.each([
    ["/inbox", "common.inbox"],
    ["/setting", "common.basic"],
  ])("labels the sidebar content section on %s", (path, label) => {
    render(
      <MemoryRouter initialEntries={[path]}>
        <AppSidebar />
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: label, level: 2 })).toBeInTheDocument();
  });

  it.each(["/inbox", "/archived"])("shows Home as the direct destination from %s", (path) => {
    authState.currentUser = { name: "users/test", username: "test" };
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

  it("keeps the mobile header limited to navigation and context", () => {
    render(
      <MemoryRouter initialEntries={["/about"]}>
        <MobileAppHeader />
      </MemoryRouter>,
    );

    expect(screen.getByRole("button", { name: "common.open-navigation" })).toHaveAttribute("data-mobile-navigation-trigger");
    const mobileBrand = screen.getByRole("button", { name: "space.switch: common.memos" });
    expect(mobileBrand).toHaveClass("h-9", "gap-1.5", "px-1");
    expect(screen.queryByRole("button", { name: "common.search" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "editor.new-memo" })).not.toBeInTheDocument();
  });

  it("exposes Compose in the mobile navigation drawer", () => {
    sidebarState.mobileOpen = true;
    render(
      <MemoryRouter initialEntries={["/about"]}>
        <MobileAppSidebar />
      </MemoryRouter>,
    );

    expect(screen.getByRole("button", { name: "common.search" })).toBeInTheDocument();
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
