import { act, fireEvent, render, screen } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import StatisticsView from "@/components/StatisticsView";
import { MemoFilterProvider } from "@/contexts/MemoFilterContext";

vi.mock("@/contexts/InstanceContext", () => ({ useInstance: () => ({ generalSetting: { weekStartDayOffset: 0 } }) }));

const dayFilter = (day: string) => `/?filter=displayTime%3A${day}`;

function renderCalendar(...locations: string[]) {
  const router = createMemoryRouter(
    [
      {
        path: "/",
        element: (
          <MemoFilterProvider>
            <StatisticsView statisticsData={{ activityStats: {}, timeBasis: "create_time" }} />
          </MemoFilterProvider>
        ),
      },
    ],
    { initialEntries: locations },
  );
  render(<RouterProvider router={router} />);
  return router;
}

describe("Sidebar calendar month", () => {
  it("opens to the selected day in a restored URL", () => {
    renderCalendar(dayFilter("2024-04-25"));

    expect(screen.getByRole("group", { name: "Calendar for 2024-04" })).toBeVisible();
    expect(screen.getByRole("button", { name: "2024-04-25 (selected)" })).toBeVisible();
  });

  it("keeps manual browsing until history restores another day, even within the same month", async () => {
    const router = renderCalendar(dayFilter("2024-04-25"), dayFilter("2024-04-26"));

    fireEvent.click(screen.getByRole("button", { name: "Next month" }));
    expect(screen.getByRole("group", { name: "Calendar for 2024-05" })).toBeVisible();
    expect(router.state.location.search).toBe("?filter=displayTime%3A2024-04-26");

    await act(() => router.navigate(-1));
    expect(screen.getByRole("button", { name: "2024-04-25 (selected)" })).toBeVisible();

    await act(() => router.navigate(dayFilter("2024-06-10")));
    expect(screen.getByRole("group", { name: "Calendar for 2024-06" })).toBeVisible();
    expect(screen.getByRole("button", { name: "2024-06-10 (selected)" })).toBeVisible();
  });

  it("keeps the browsed month when the day filter is cleared or invalid", async () => {
    const router = renderCalendar(dayFilter("2024-04-25"));

    fireEvent.click(screen.getByRole("button", { name: "Previous month" }));
    await act(() => router.navigate("/"));
    expect(screen.getByRole("group", { name: "Calendar for 2024-03" })).toBeVisible();

    await act(() => router.navigate(dayFilter("2024-02-30")));
    expect(screen.getByRole("group", { name: "Calendar for 2024-03" })).toBeVisible();
    expect(screen.queryByRole("button", { name: /selected/ })).toBeNull();
  });
});
