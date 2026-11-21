import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { MemoFilterProvider } from "@/contexts/MemoFilterContext";
import { useDateFilterNavigation } from "@/hooks/useDateFilterNavigation";

const Harness = ({ destination }: { destination?: string }) => {
  const location = useLocation();
  const navigateToDate = useDateFilterNavigation(destination);
  return (
    <>
      <output data-testid="search">{location.search}</output>
      <output data-testid="pathname">{location.pathname}</output>
      <button type="button" onClick={() => navigateToDate("2026-08-02")}>
        Select date
      </button>
    </>
  );
};

describe("useDateFilterNavigation", () => {
  it("preserves unrelated query parameters while replacing the date filter", async () => {
    render(
      <MemoryRouter initialEntries={["/?creator=steven&sort=displayTime&filter=tagSearch%3Awork"]}>
        <MemoFilterProvider>
          <Harness />
        </MemoFilterProvider>
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Select date" }));

    await waitFor(() => {
      const params = new URLSearchParams(screen.getByTestId("search").textContent ?? "");
      expect(params.get("sort")).toBe("displayTime");
      expect(params.get("creator")).toBe("steven");
      expect(params.get("filter")).toContain("tagSearch:work");
      expect(params.get("filter")).toContain("displayTime:2026-08-02");
    });
  });

  it.each(["/", "/archived"])("opens %s with the selected date from the trash", (destination) => {
    render(
      <MemoryRouter initialEntries={["/trash"]}>
        <MemoFilterProvider>
          <Harness destination={destination} />
        </MemoFilterProvider>
      </MemoryRouter>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Select date" }));
    expect(screen.getByTestId("pathname")).toHaveTextContent(destination);
    expect(screen.getByTestId("search")).toHaveTextContent("?filter=displayTime%3A2026-08-02");
  });
});
