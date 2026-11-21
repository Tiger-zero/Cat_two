import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SIDEBAR_ROW_BOX_CLASSES, SIDEBAR_ROW_COUNT_RAIL_CLASSES, SIDEBAR_ROW_SLOT_CLASSES } from "@/components/AppSidebar/SidebarRow";
import TagsSection from "@/components/AppSidebar/TagsSection";
import { MemoFilterProvider, parseFilterQuery } from "@/contexts/MemoFilterContext";

const mocks = vi.hoisted(() => ({ account: "users/alice", authenticatedFetch: vi.fn(), toast: vi.fn() }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => ({ name: mocks.account }) }));
vi.mock("@/connect", () => ({ authenticatedFetch: mocks.authenticatedFetch }));
vi.mock("@/lib/toast", () => ({ toast: { add: mocks.toast }, showErrorToast: vi.fn() }));
vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string, options?: { tag?: string }) => (options?.tag ? `${key} ${options.tag}` : key),
}));

const FilterQuery = () => <output data-testid="search">{useLocation().search}</output>;
const selectedFilters = () => parseFilterQuery(new URLSearchParams(screen.getByTestId("search").textContent ?? "").get("filter"));

describe("TagsSection", () => {
  beforeEach(() => {
    localStorage.clear();
    mocks.account = "users/alice";
  });

  it("keeps the title count-free and uses the shared section action grammar", () => {
    render(
      <MemoryRouter>
        <MemoFilterProvider>
          <TagsSection tagCount={{ a: 2, "a/b": 1 }} scope="home" />
        </MemoFilterProvider>
      </MemoryRouter>,
    );

    const heading = screen.getByRole("heading", { name: "common.tags", level: 2 });
    expect(heading.parentElement).toHaveTextContent(/^common.tags$/);

    const trigger = screen.getByRole("button", { name: "common.tags: common.more" });
    expect(trigger).toHaveClass("mu-button", "mu-button-ghost", "mu-button-icon", "mu-s-xs");
    expect(trigger.querySelector("svg")).not.toBeNull();
    expect(screen.queryByRole("button", { name: "common.tags: memo.layout-list" })).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitemcheckbox")).not.toBeInTheDocument();
  });

  it("switches layouts from the menu, closes it, and persists the preference", async () => {
    const onSelect = vi.fn();
    render(
      <MemoryRouter>
        <MemoFilterProvider>
          <TagsSection tagCount={{ a: 2, "a/b": 1 }} scope="home" onSelect={onSelect} />
        </MemoFilterProvider>
      </MemoryRouter>,
    );

    const trigger = screen.getByRole("button", { name: "common.tags: common.more" });
    fireEvent.click(trigger);
    const treeMode = await screen.findByRole("menuitemcheckbox", { name: "common.tree-mode" });
    expect(treeMode).toHaveAttribute("aria-checked", "true");
    fireEvent.click(treeMode);

    expect(screen.queryByRole("tree")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "#a/b, setting.tags.used-count" })).toBeInTheDocument();
    expect(localStorage.getItem("tag-view-as-tree")).toBe("false");
    await waitFor(() => expect(screen.queryByRole("menuitemcheckbox")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());

    fireEvent.click(trigger);
    const checkedTreeMode = await screen.findByRole("menuitemcheckbox", { name: "common.tree-mode" });
    expect(checkedTreeMode).toHaveAttribute("aria-checked", "false");
    fireEvent.click(checkedTreeMode);

    expect(screen.getByRole("tree")).toBeInTheDocument();
    expect(localStorage.getItem("tag-view-as-tree")).toBe("true");
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("reflects an existing tree preference when opening the menu", async () => {
    localStorage.setItem("tag-view-as-tree", "true");
    render(
      <MemoryRouter>
        <MemoFilterProvider>
          <TagsSection tagCount={{ a: 2, "a/b": 1 }} scope="home" />
        </MemoFilterProvider>
      </MemoryRouter>,
    );

    expect(screen.getByRole("tree")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "common.tags: common.more" }));
    expect(await screen.findByRole("menuitemcheckbox", { name: "common.tree-mode" })).toHaveAttribute("aria-checked", "true");
  });

  it("keeps flat rows on the shared row grammar with a trailing count rail", () => {
    localStorage.setItem("tag-view-as-tree", "false");
    render(
      <MemoryRouter>
        <MemoFilterProvider>
          <TagsSection tagCount={{ alpha: 2, "a/very-long-tag-path": 1 }} scope="home" />
        </MemoFilterProvider>
      </MemoryRouter>,
    );

    const alpha = screen.getByText("alpha");
    const alphaButton = alpha.closest("button") as HTMLButtonElement;
    const path = alpha.parentElement?.parentElement as HTMLSpanElement;
    const count = screen.getByText("2");

    expect(alphaButton.parentElement).toHaveClass(...SIDEBAR_ROW_BOX_CLASSES.split(" "));
    // The # sits in the same fixed slot the tree uses, so switching modes keeps it in place.
    expect(alphaButton.firstElementChild).toHaveClass(...SIDEBAR_ROW_SLOT_CLASSES.split(" "));
    expect(path).toHaveClass("truncate", "text-start");
    expect(path).not.toContainElement(count);
    expect(count).toHaveClass(...SIDEBAR_ROW_COUNT_RAIL_CLASSES.split(" "));
  });

  it("moves a nested pin above the ordinary tree, restores it per account, and unpins it", async () => {
    const tags = () => (
      <MemoryRouter>
        <MemoFilterProvider>
          <TagsSection tagCount={{ alpha: 1, a: 2, "a/b": 1 }} scope="home" />
        </MemoFilterProvider>
      </MemoryRouter>
    );
    const { rerender } = render(tags());

    fireEvent.click(screen.getByRole("button", { name: "common.expand #a" }));
    fireEvent.click(screen.getByRole("button", { name: "setting.tags.actions #a/b" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "common.pin" }));

    const pinned = screen.getByRole("tree", { name: "setting.tags.pinned" });
    expect(
      pinned.compareDocumentPosition(screen.getByRole("tree", { name: "common.tags" })) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "#a/b, setting.tags.used-count" })).toHaveLength(1);
    expect(localStorage.getItem("memos-pinned-tags:users/alice")).toBe('["a/b"]');

    mocks.account = "users/bob";
    rerender(tags());
    expect(screen.queryByRole("tree", { name: "setting.tags.pinned" })).not.toBeInTheDocument();
    mocks.account = "users/alice";
    rerender(tags());

    fireEvent.click(
      within(screen.getByRole("tree", { name: "setting.tags.pinned" })).getByRole("button", { name: "setting.tags.actions #a/b" }),
    );
    fireEvent.click(await screen.findByRole("menuitem", { name: "common.unpin" }));
    expect(screen.queryByRole("tree", { name: "setting.tags.pinned" })).not.toBeInTheDocument();
    expect(localStorage.getItem("memos-pinned-tags:users/alice")).toBe("[]");
  });

  it("selects the full nested path without dropping the current date filter", async () => {
    render(
      <MemoryRouter initialEntries={["/?filter=displayTime%3A2027-08-01"]}>
        <MemoFilterProvider>
          <TagsSection tagCount={{ 一级入口: 2, "一级入口/二级入口": 2, "一级入口/二级入口/笔记": 1 }} scope="home" />
          <FilterQuery />
        </MemoFilterProvider>
      </MemoryRouter>,
    );
    fireEvent.click(screen.getByRole("button", { name: "common.expand #一级入口" }));
    fireEvent.click(screen.getByRole("button", { name: "#一级入口/二级入口, setting.tags.used-count" }));

    await waitFor(() =>
      expect(selectedFilters()).toEqual([
        { factor: "displayTime", value: "2027-08-01" },
        { factor: "tagSearch", value: "一级入口/二级入口" },
      ]),
    );
  });

  it("merges through the authenticated rename endpoint and follows the new path and pin", async () => {
    localStorage.setItem("memos-pinned-tags:users/alice", '["2027/08","2027/09"]');
    mocks.authenticatedFetch.mockResolvedValue(Response.json({ updatedCount: 2 }));
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={["/?filter=tagSearch%3A2027%2F08"]}>
          <MemoFilterProvider>
            <TagsSection tagCount={{ 2027: 3, "2027/08": 2, "2027/09": 1 }} scope="home" />
            <FilterQuery />
          </MemoFilterProvider>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "setting.tags.actions #2027/08" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "setting.tags.rename" }));
    const dialog = await screen.findByRole("dialog", { name: "setting.tags.rename-title #2027/08" });
    const input = within(dialog).getByRole("textbox", { name: "setting.tags.new-name" });
    const save = within(dialog).getByRole("button", { name: "common.save" });
    fireEvent.change(input, { target: { value: "2027/ 09" } });
    expect(save).toBeDisabled();
    fireEvent.change(input, { target: { value: "#2027/09" } });
    fireEvent.click(save);

    await waitFor(() => expect(selectedFilters()).toEqual([{ factor: "tagSearch", value: "2027/09" }]));
    expect(mocks.authenticatedFetch).toHaveBeenCalledWith("/api/tags/rename", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ oldName: "2027/08", newName: "2027/09" }),
    });
    expect(localStorage.getItem("memos-pinned-tags:users/alice")).toBe('["2027/09"]');
    expect(mocks.toast).toHaveBeenCalledWith({ title: "setting.tags.updated-notes", type: "success" });
  });
});
