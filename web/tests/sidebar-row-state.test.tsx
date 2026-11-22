import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { HashIcon } from "lucide-react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import SidebarRow, { sidebarRowStateClasses } from "@/components/AppSidebar/SidebarRow";
import TagsSection from "@/components/AppSidebar/TagsSection";
import MemoFilters from "@/components/MemoFilters";
import { AppSidebarProvider } from "@/contexts/AppSidebarContext";
import { MemoFilterProvider } from "@/contexts/MemoFilterContext";

vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => ({ name: "users/1" }) }));

const tokens = (classes: string) => classes.split(" ");

describe("sidebar selected grammar", () => {
  it("uses a green fill for a checked tag and the theme surface for current navigation", () => {
    expect(tokens(sidebarRowStateClasses("current"))).toContain("bg-mu-hover");
    expect(tokens(sidebarRowStateClasses("checked"))).toContain("bg-(--memos-sidebar-selected)");
    // Selection never changes weight, so labels do not shift.
    for (const state of ["idle", "current", "checked"] as const) {
      expect(tokens(sidebarRowStateClasses(state))).not.toContain("font-medium");
    }
  });

  it("marks a checked row so its icon and count are readable on green", () => {
    render(<SidebarRow state="checked" icon={HashIcon} label="work" count={4} />);

    const row = screen.getByRole("button", { name: "work4" });
    expect(row).toHaveAttribute("data-checked");
    expect(row).toHaveAttribute("aria-pressed", "true");
    expect(row).toHaveClass("bg-(--memos-sidebar-selected)", "text-(--memos-on-sidebar-selected)");
    expect(row.querySelector("svg")).toHaveClass("group-data-checked:text-(--memos-on-sidebar-selected)");
    expect(screen.getByText("4")).toHaveClass("group-data-checked:text-(--memos-on-sidebar-selected)");
  });

  it("fills the selected tag row", () => {
    localStorage.removeItem("tag-view-as-tree");
    render(
      <MemoryRouter>
        <MemoFilterProvider>
          <TagsSection tagCount={{ work: 2 }} scope="home" />
        </MemoFilterProvider>
      </MemoryRouter>,
    );

    const row = screen.getByRole("button", { name: "#work, setting.tags.used-count" });
    const treeItem = screen.getByRole("treeitem");
    expect(treeItem).not.toHaveAttribute("data-checked");
    fireEvent.click(row);
    expect(treeItem).toHaveAttribute("data-checked");
    expect(treeItem).toHaveClass("bg-(--memos-sidebar-selected)", "text-(--memos-on-sidebar-selected)");
    expect(row).toHaveAttribute("aria-pressed", "true");
  });

  it.each(["/", "/archived"])("returns from trash to %s when selecting a tag", (destination) => {
    const LocationProbe = () => {
      const { pathname, search } = useLocation();
      return <output data-testid="tag-location">{`${pathname}${search}`}</output>;
    };
    render(
      <MemoryRouter initialEntries={["/trash"]}>
        <MemoFilterProvider>
          <TagsSection tagCount={{ work: 2 }} scope="home" destination={destination} />
          <LocationProbe />
        </MemoFilterProvider>
      </MemoryRouter>,
    );
    const row = screen.getByRole("button", { name: "#work, setting.tags.used-count" });
    fireEvent.click(row);
    expect(screen.getByTestId("tag-location")).toHaveTextContent(`${destination}?filter=tagSearch%3Awork`);
    expect(row).toHaveAttribute("aria-pressed", "true");
  });
});

describe("MemoFilters", () => {
  it("formats a day filter as a date rather than the raw value", () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={["/?filter=displayTime:2026-09-02"]}>
          <AppSidebarProvider>
            <MemoFilterProvider>
              <MemoFilters />
            </MemoFilterProvider>
          </AppSidebarProvider>
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(screen.getByText("Sep 2, 2026")).toBeInTheDocument();
  });
});
