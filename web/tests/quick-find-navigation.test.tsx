import { act, createEvent, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import QuickFindDialog from "@/components/AppSidebar/QuickFindDialog";
import MemoFilters from "@/components/MemoFilters";
import { AppSidebarProvider, useAppSidebar } from "@/contexts/AppSidebarContext";
import { getFilterSearch, type MemoFilter, parseFilterQuery } from "@/contexts/MemoFilterContext";

const state = vi.hoisted(() => ({
  currentUser: { name: "users/alice" } as { name: string } | undefined,
  filters: [] as MemoFilter[],
  setFilters: vi.fn(),
  removeFilter: vi.fn(),
}));

vi.mock("@/contexts/MemoFilterContext", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/contexts/MemoFilterContext")>();
  return {
    ...actual,
    useMemoFilterContext: () => ({
      filters: state.filters,
      setFilters: state.setFilters,
      removeFilter: state.removeFilter,
    }),
  };
});

vi.mock("@/hooks/useCurrentUser", () => ({
  default: () => state.currentUser,
}));

vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string) => key,
}));

const Harness = () => {
  const location = useLocation();
  const { setQuickFindOpen } = useAppSidebar();

  return (
    <>
      <output data-testid="path">{`${location.pathname}${location.search}`}</output>
      <button type="button" onClick={() => setQuickFindOpen(true)}>
        Open Quick Find
      </button>
      <QuickFindDialog />
      <MemoFilters />
    </>
  );
};

describe("Quick Find navigation", () => {
  beforeEach(() => {
    sessionStorage.clear();
    state.currentUser = { name: "users/alice" };
    state.filters = [];
    state.setFilters.mockClear();
  });

  const renderSearch = (initialEntry = "/") => {
    const router = createMemoryRouter(
      [
        {
          path: "*",
          element: (
            <AppSidebarProvider>
              <Harness />
            </AppSidebarProvider>
          ),
        },
      ],
      { initialEntries: [initialEntry] },
    );
    render(<RouterProvider router={router} />);
    return router;
  };
  const openQuickFind = () => fireEvent.click(screen.getByRole("button", { name: "Open Quick Find" }));

  it("submits a text query from the input's search action", async () => {
    renderSearch();
    openQuickFind();
    fireEvent.change(await screen.findByRole("textbox"), { target: { value: "roadmap" } });
    fireEvent.click(screen.getByRole("button", { name: "common.search" }));

    expect(state.setFilters).toHaveBeenCalledWith([{ factor: "contentSearch", value: "roadmap" }]);
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("preserves the draft when changing modes and submits CEL with Enter", async () => {
    renderSearch();
    openQuickFind();
    fireEvent.change(await screen.findByRole("textbox"), { target: { value: "pinned || has_link" } });
    fireEvent.click(screen.getByRole("button", { name: "search.expression-mode" }));
    const expression = screen.getByRole("textbox");
    expect(expression.tagName).toBe("TEXTAREA");
    expect(expression).toHaveValue("pinned || has_link");
    expect(expression).toHaveFocus();
    fireEvent.keyDown(expression, { key: "Enter", shiftKey: true });
    expect(state.setFilters).not.toHaveBeenCalled();
    fireEvent.keyDown(expression, { key: "Enter" });
    expect(state.setFilters).toHaveBeenCalledWith([{ factor: "celSearch", value: "pinned || has_link" }]);
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("moves keyboard focus across the mode switch without losing the draft", async () => {
    renderSearch();
    openQuickFind();
    fireEvent.change(await screen.findByRole("textbox"), { target: { value: "pinned" } });
    const textMode = screen.getByRole("button", { name: "search.text" });
    act(() => textMode.focus());
    fireEvent.keyDown(textMode, { key: "ArrowRight" });
    const expressionMode = screen.getByRole("button", { name: "search.expression-mode" });
    await waitFor(() => expect(expressionMode).toHaveFocus());
    // Arrow keys move focus; pressing a mode chooses it.
    expect(textMode).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(expressionMode);
    expect(expressionMode).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("textbox")).toHaveValue("pinned");
  });

  it("restores the expression from its chip and discards cancelled changes", async () => {
    state.filters = [
      { factor: "celSearch", value: "pinned\n || has_code" },
      { factor: "tagSearch", value: "work" },
    ];
    renderSearch();
    openQuickFind();
    const expression = await screen.findByRole("textbox");
    expect(expression).toHaveValue("pinned\n || has_code");
    expect(screen.getByRole("button", { name: "search.expression-mode" })).toHaveAttribute("aria-pressed", "true");
    fireEvent.change(expression, { target: { value: "has_link" } });
    fireEvent.keyDown(expression, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(state.setFilters).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "search.edit-query" }));
    expect(await screen.findByRole("textbox")).toHaveValue("pinned\n || has_code");
    fireEvent.click(screen.getByRole("button", { name: "search.text" }));
    expect(screen.getByRole("textbox")).toHaveValue("pinned || has_code");
    fireEvent.click(screen.getByRole("button", { name: "search.expression-mode" }));
    expect(screen.getByRole("textbox")).toHaveValue("pinned\n || has_code");
  });

  it.each(["text", "cel"])("ignores Enter during IME composition in %s mode", async (mode) => {
    renderSearch();
    openQuickFind();
    if (mode === "cel") fireEvent.click(screen.getByRole("button", { name: "search.expression-mode" }));
    const input = await screen.findByRole("textbox");
    fireEvent.change(input, { target: { value: "pinned" } });
    const composingEnter = createEvent.keyDown(input, { key: "Enter", isComposing: true });
    const legacyComposingEnter = createEvent.keyDown(input, { key: "Enter", keyCode: 229 });
    fireEvent(input, composingEnter);
    fireEvent(input, legacyComposingEnter);
    expect(composingEnter.defaultPrevented).toBe(false);
    expect(legacyComposingEnter.defaultPrevented).toBe(false);
    expect(state.setFilters).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("removes only CEL using the chip remove button", async () => {
    state.filters = [
      { factor: "celSearch", value: "pinned" },
      { factor: "tagSearch", value: "work" },
    ];
    renderSearch();
    openQuickFind();
    fireEvent.keyDown(await screen.findByRole("textbox"), { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    const [removeCel] = screen.getAllByRole("button", { name: "memo.filters.remove" });
    fireEvent.click(removeCel);
    const predicate = state.removeFilter.mock.calls[0][0];
    expect(state.filters.filter((filter) => !predicate(filter))).toEqual([{ factor: "tagSearch", value: "work" }]);
  });

  it("searches from Settings into the personal feed in one history step", async () => {
    const router = renderSearch("/setting");
    openQuickFind();
    const input = await screen.findByRole("textbox");
    fireEvent.change(input, { target: { value: "roadmap" } });
    fireEvent.submit(input.closest("form")!);

    await waitFor(() => expect(screen.getByTestId("path")).toHaveTextContent("/?filter=contentSearch%3Aroadmap"));
    expect(state.setFilters).not.toHaveBeenCalled();

    await act(async () => {
      await router.navigate(-1);
    });

    expect(screen.getByTestId("path")).toHaveTextContent("/setting");
  });

  it.each([
    ["/", "/archived", "search.unarchived", "common.archived"],
    ["/archived", "/", "common.archived", "search.unarchived"],
  ])("can switch the search scope from %s to %s", async (from, to, defaultLabel, targetLabel) => {
    const facets: MemoFilter[] = [
      { factor: "tagSearch", value: "work/2026" },
      { factor: "displayTime", value: "2026-11-21" },
    ];
    state.filters = [...facets, { factor: "contentSearch", value: "previous" }];
    const initialEntry = `${from}${getFilterSearch(state.filters)}`;
    const router = renderSearch(initialEntry);

    openQuickFind();
    const group = within(await screen.findByRole("group", { name: "search.scope" }));
    expect(group.getByRole("button", { name: defaultLabel })).toHaveAttribute("aria-pressed", "true");
    const input = screen.getByRole("textbox");
    fireEvent.change(input, { target: { value: "roadmap" } });
    fireEvent.click(group.getByRole("button", { name: targetLabel }));
    expect(input).toHaveValue("roadmap");
    fireEvent.keyDown(input, { key: "Enter" });

    expect(router.state.location.pathname).toBe(to);
    expect(parseFilterQuery(new URLSearchParams(router.state.location.search).get("filter"))).toEqual([
      ...facets,
      { factor: "contentSearch", value: "roadmap" },
    ]);
    expect(state.setFilters).not.toHaveBeenCalled();

    await act(async () => {
      await router.navigate(-1);
    });
    expect(screen.getByTestId("path")).toHaveTextContent(initialEntry);
  });

  it("preserves an anonymous global page in history", async () => {
    state.currentUser = undefined;

    const router = renderSearch("/about");

    openQuickFind();
    const input = await screen.findByRole("textbox");
    expect(screen.queryByRole("group", { name: "search.scope" })).not.toBeInTheDocument();
    fireEvent.change(input, { target: { value: "roadmap" } });
    fireEvent.submit(input.closest("form")!);

    await waitFor(() => expect(screen.getByTestId("path")).toHaveTextContent("/?filter=contentSearch%3Aroadmap"));

    await act(async () => {
      await router.navigate(-1);
    });

    expect(screen.getByTestId("path")).toHaveTextContent("/about");
  });
});
