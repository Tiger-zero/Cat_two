import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { CalendarCell } from "@/components/ActivityCalendar/CalendarCell";
import { MonthCalendar } from "@/components/ActivityCalendar/MonthCalendar";
import type { CalendarDayCell } from "@/components/ActivityCalendar/types";

vi.mock("@/contexts/InstanceContext", () => ({ useInstance: () => ({ generalSetting: { weekStartDayOffset: 0 } }) }));

const makeDay = (overrides: Partial<CalendarDayCell> = {}): CalendarDayCell => ({
  date: "2025-05-01",
  label: 1,
  count: 0,
  isCurrentMonth: true,
  isToday: false,
  isSelected: false,
  ...overrides,
});

/** The button spans its column; the square chip inside it carries every visual. */
const chipOf = (button: HTMLElement) => button.firstElementChild as HTMLElement;

describe("CalendarCell empty-day clickability", () => {
  it("fires onClick for an in-month day with count=0", () => {
    const onClick = vi.fn();
    render(<CalendarCell day={makeDay()} tooltipText="May 1, 2025" onClick={onClick} />);

    const button = screen.getByRole("button", { name: /May 1, 2025/ });
    fireEvent.click(button);

    expect(onClick).toHaveBeenCalledWith("2025-05-01");
  });

  it("renders an empty in-month day as interactive (tabIndex 0, not aria-disabled)", () => {
    render(<CalendarCell day={makeDay()} tooltipText="May 1, 2025" onClick={() => {}} />);

    const button = screen.getByRole("button", { name: /May 1, 2025/ });
    expect(button).toHaveAttribute("tabindex", "0");
    expect(button).toHaveAttribute("aria-disabled", "false");
    expect(button).not.toHaveClass("mu-tooltip-trigger");
    expect(chipOf(button)).toHaveClass("bg-[var(--memos-activity-empty,var(--mu-well))]");
  });

  it("still renders a populated in-month day as interactive", () => {
    const onClick = vi.fn();
    render(<CalendarCell day={makeDay({ count: 3 })} tooltipText="May 1, 2025" onClick={onClick} />);

    const button = screen.getByRole("button", { name: /May 1, 2025/ });
    expect(button).toHaveClass("mu-tooltip-trigger");
    fireEvent.click(button);
    expect(onClick).toHaveBeenCalledWith("2025-05-01");
  });

  it.each([
    [1, "low"],
    [2, "low"],
    [3, "medium"],
    [5, "medium"],
    [6, "high"],
  ] as const)("colors %s notes in the %s activity band", (count, band) => {
    render(<CalendarCell day={makeDay({ count })} tooltipText="May 1, 2025" onClick={() => {}} />);
    expect(chipOf(screen.getByRole("button"))).toHaveClass(`bg-[var(--memos-activity-${band},var(--mu-selected))]`);
  });

  it("marks today with a green border and a dot without changing the numeral weight", () => {
    render(<CalendarCell day={makeDay({ isToday: true })} tooltipText="May 1, 2025" onClick={() => {}} />);

    const button = screen.getByRole("button", { name: /May 1, 2025/ });
    expect(button).not.toHaveClass("font-semibold", "font-bold");
    expect(chipOf(button)).toHaveClass("border-[color:var(--memos-activity-today,var(--mu-primary))]");
    expect(button.querySelector('[aria-hidden="true"]')).toHaveClass("rounded-full");
  });

  it.each([0, 3])("keeps an unnumbered day with %i notes clickable and accessible without a tooltip", (count) => {
    const onClick = vi.fn();
    render(<CalendarCell day={makeDay({ count, isToday: true })} tooltipText="2025-05-01" showDateLabels={false} onClick={onClick} />);

    const button = screen.getByRole("button", { name: "2025-05-01" });
    expect(button).toHaveAttribute("aria-current", "date");
    expect(button).toHaveAttribute("tabindex", "0");
    expect(button).not.toHaveClass("mu-tooltip-trigger");
    expect(button).not.toHaveAttribute("title");
    expect(chipOf(button)).toHaveClass("border-[color:var(--memos-activity-today,var(--mu-primary))]");
    expect(chipOf(button).firstElementChild).toHaveClass("mu-visually-hidden");
    expect(button.querySelector('[aria-hidden="true"]')).toBeNull();
    fireEvent.click(button);
    expect(onClick).toHaveBeenCalledWith("2025-05-01");
  });

  it("marks the selected filter with a soft fill and accent border", () => {
    render(<CalendarCell day={makeDay({ isSelected: true })} tooltipText="May 1, 2025" onClick={() => {}} />);

    const button = screen.getByRole("button", { name: /selected/ });
    expect(chipOf(button)).toHaveClass("bg-mu-selected", "text-mu-text", "border-mu-accent");
    expect(chipOf(button)).not.toHaveClass("font-medium");
    expect(chipOf(button)).not.toHaveClass("ring-2", "ring-inset");
    expect(chipOf(button)).not.toHaveClass("font-semibold", "font-bold");
  });

  it("keeps the accent fill on a selected empty day under hover", () => {
    render(<CalendarCell day={makeDay({ isSelected: true })} tooltipText="May 1, 2025" onClick={() => {}} />);

    const chip = chipOf(screen.getByRole("button", { name: /selected/ }));
    expect(chip).not.toHaveClass("bg-[var(--memos-activity-empty,var(--mu-well))]");
    expect(chip).toHaveClass("bg-mu-selected");
  });

  it("caps the chip so a wider container buys hit area, not calendar height", () => {
    render(<CalendarCell day={makeDay()} tooltipText="May 1, 2025" onClick={() => {}} />);

    const button = screen.getByRole("button", { name: /May 1, 2025/ });
    // The square lives on the capped chip. Putting it back on the button would make row
    // height track the column width again, which is what made a widened rail so tall.
    expect(button).not.toHaveClass("aspect-square");
    expect(chipOf(button)).toHaveClass("aspect-square", "max-w-7");
  });

  it("does not render out-of-month days as interactive (no role=button)", () => {
    render(<CalendarCell day={makeDay({ isCurrentMonth: false })} tooltipText="May 1, 2025" onClick={() => {}} />);

    expect(screen.queryByRole("button")).toBeNull();
  });
});

describe("MonthCalendar date labels", () => {
  it("omits the weekday row and outside dates in numberless mode, with the same day grid and selection", () => {
    const selectedDate = "2026-11-24";
    const props = { month: "2026-11", data: {}, selectedDate, onClick: vi.fn() };
    const { rerender } = render(<MonthCalendar {...props} showDateLabels={false} />);

    const month = screen.getByRole("group");
    const grid = month.firstElementChild!;
    expect(month.children).toHaveLength(1);
    expect(grid.children).toHaveLength(35);
    expect(grid.lastElementChild?.textContent).toBe("");
    const dayButtons = screen.getAllByRole("button");
    expect(dayButtons).toHaveLength(30);
    expect(dayButtons.every((button) => chipOf(button).querySelector(".mu-visually-hidden"))).toBe(true);
    expect(chipOf(screen.getByRole("button", { name: `${selectedDate} (selected)` }))).toHaveClass("bg-mu-selected");

    rerender(<MonthCalendar {...props} />);
    expect(month.children).toHaveLength(2);
    expect(month.firstElementChild?.children).toHaveLength(7);
    expect(screen.getAllByRole("button")).toHaveLength(30);
    expect(chipOf(screen.getByRole("button", { name: `${selectedDate} (selected)` }))).toHaveTextContent("24");
  });
});
