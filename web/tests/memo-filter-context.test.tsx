import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider, useLocation } from "react-router-dom";
import { describe, expect, it } from "vitest";
import {
  type MemoFilter,
  MemoFilterProvider,
  parseFilterQuery,
  stringifyFilters,
  useMemoFilterContext,
} from "@/contexts/MemoFilterContext";

const expression = 'tags.exists(t, t.contains("50%, café & C++"))\n || space == null';

const Harness = () => {
  const { filters, setFilters, addFilter, removeFiltersByFactor } = useMemoFilterContext();
  const location = useLocation();
  return (
    <div>
      <output data-testid="filters">{JSON.stringify(filters)}</output>
      <output data-testid="url">{location.search}</output>
      <button type="button" onClick={() => setFilters([{ factor: "celSearch", value: expression }])}>
        Search CEL
      </button>
      <button type="button" onClick={() => addFilter({ factor: "tagSearch", value: "work" })}>
        Select work
      </button>
      <button type="button" onClick={() => removeFiltersByFactor("untagged")}>
        Clear untagged
      </button>
    </div>
  );
};

describe("MemoFilterProvider", () => {
  it("restores CEL after URL synchronization, navigation history, and remount", async () => {
    const routes = [
      {
        path: "*",
        element: (
          <MemoFilterProvider>
            <Harness />
          </MemoFilterProvider>
        ),
      },
    ];
    const router = createMemoryRouter(routes, { initialEntries: ["/inbox", "/explore"], initialIndex: 1 });
    const rendered = render(<RouterProvider router={router} />);
    fireEvent.click(screen.getByRole("button", { name: "Search CEL" }));
    const expected: MemoFilter[] = [{ factor: "celSearch", value: expression }];
    await waitFor(() => expect(parseFilterQuery(new URLSearchParams(router.state.location.search).get("filter"))).toEqual(expected));
    const savedUrl = `/explore${router.state.location.search}`;
    await act(async () => {
      await router.navigate(-1);
    });
    await waitFor(() => expect(screen.getByTestId("filters")).toHaveTextContent("[]"));
    await act(async () => {
      await router.navigate(1);
    });
    await waitFor(() => expect(screen.getByTestId("filters")).toHaveTextContent(JSON.stringify(expected)));
    rendered.unmount();
    render(<RouterProvider router={createMemoryRouter(routes, { initialEntries: [savedUrl] })} />);
    expect(screen.getByTestId("filters")).toHaveTextContent(JSON.stringify(expected));
    expect(new URLSearchParams(router.state.location.search).get("filter")).toBe(stringifyFilters(expected));
  });
  it("preserves the Space origin when navigation removes collection filters", async () => {
    const origin = "/spaces/product?filter=tagSearch%3Awork";
    const router = createMemoryRouter(
      [
        {
          path: "*",
          element: (
            <MemoFilterProvider>
              <Harness />
            </MemoFilterProvider>
          ),
        },
      ],
      { initialEntries: [origin] },
    );
    render(<RouterProvider router={router} />);
    const state = { from: origin };
    await act(() => router.navigate("/memos/1", { state }));
    await waitFor(() => expect(screen.getByTestId("filters")).toHaveTextContent("[]"));
    expect(router.state.location.search).toBe("");
    expect(router.state.location.state).toEqual(state);
    await act(() => router.navigate(-1));
    expect(router.state.location.pathname + router.state.location.search).toBe(origin);
    expect(screen.getByTestId("filters")).toHaveTextContent('"work"');
  });

  it("keeps encoded values containing colons intact", () => {
    expect(parseFilterQuery("contentSearch:https://example.com:8080/path")).toEqual([
      { factor: "contentSearch", value: "https://example.com:8080/path" },
    ]);
  });

  it("restores and clears untagged from the home URL alongside the current date", async () => {
    const router = createMemoryRouter(
      [
        {
          path: "*",
          element: (
            <MemoFilterProvider>
              <Harness />
            </MemoFilterProvider>
          ),
        },
      ],
      { initialEntries: ["/?filter=displayTime%3A2026-11-21%2Cuntagged%3A"] },
    );
    const rendered = render(<RouterProvider router={router} />);
    expect(screen.getByTestId("filters")).toHaveTextContent('"untagged"');
    fireEvent.click(screen.getByRole("button", { name: "Clear untagged" }));
    expect(screen.getByTestId("filters")).not.toHaveTextContent('"untagged"');
    expect(router.state.location.pathname + router.state.location.search).toBe("/?filter=displayTime%3A2026-11-21");

    await act(() => router.navigate("/?filter=untagged%3A"));
    fireEvent.click(screen.getByRole("button", { name: "Select work" }));
    expect(screen.getByTestId("filters")).not.toHaveTextContent('"untagged"');
    expect(router.state.location.pathname + router.state.location.search).toBe("/?filter=tagSearch%3Awork");
    rendered.unmount();
  });

  it("restores the last chosen tag mode from contradictory URL facets", () => {
    expect(parseFilterQuery("tagSearch:work,untagged:")).toEqual([{ factor: "untagged", value: "" }]);
    expect(parseFilterQuery("untagged:,tagSearch:work")).toEqual([{ factor: "tagSearch", value: "work" }]);
  });
});
