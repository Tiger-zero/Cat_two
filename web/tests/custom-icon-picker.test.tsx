import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import CustomIconPicker from "@/components/CustomIconPicker";
import type { CustomIconValue } from "@/lib/custom-icons";
import { SPACE_EMOJI } from "@/lib/space-emoji";
import { FULLY_QUALIFIED_EMOJI } from "@/utils/tag-unicode-data";

vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));

const icon = (value: string, kind: "emoji" | "lucide" = "lucide"): CustomIconValue => ({ value: { case: kind, value } });
function Harness({ initial, onChange = vi.fn() }: { initial?: CustomIconValue; onChange?: (value?: CustomIconValue) => void }) {
  const [value, setValue] = useState(initial);
  return (
    <CustomIconPicker
      label="Choose icon"
      value={value}
      onChange={(next) => {
        setValue(next);
        onChange(next);
      }}
    />
  );
}

// Base UI treats a change as typing (and highlights the first match) only when it carries an inputType.
const type = (input: HTMLElement, value: string, inputType = "insertText") => fireEvent.input(input, { target: { value }, inputType });

describe("CustomIconPicker", () => {
  it("searches and selects a symbol with keyboard navigation and restores trigger focus", async () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    const trigger = screen.getByRole("button", { name: "Choose icon" });
    fireEvent.click(trigger);
    const search = await screen.findByRole("combobox", { name: "space.icon.search-icons" });
    await waitFor(() => expect(search).toHaveFocus());
    type(search, "book");
    // The search keeps focus; arrow keys move the grid's highlight.
    await waitFor(() => expect(screen.getByRole("gridcell", { name: "Book" })).toHaveAttribute("data-highlighted"));
    fireEvent.keyDown(search, { key: "ArrowRight" });
    await waitFor(() => expect(screen.getByRole("gridcell", { name: "Notebook" })).toHaveAttribute("data-highlighted"));
    expect(search).toHaveFocus();
    fireEvent.click(screen.getByRole("gridcell", { name: "Notebook" }));
    expect(onChange).toHaveBeenCalledWith(icon("notebook-pen"));
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it("accepts a pasted joined emoji and can reset it", async () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: "Choose icon" }));
    fireEvent.click(await screen.findByRole("tab", { name: "space.icon.emoji" }));
    const search = screen.getByRole("combobox", { name: "space.icon.search-emoji" });
    search.focus();
    type(search, "👩🏽‍🌾", "insertFromPaste");
    await waitFor(() => expect(screen.getByRole("gridcell", { name: "space.icon.use-emoji" })).toHaveAttribute("data-highlighted"));
    fireEvent.keyDown(search, { key: "Enter" });
    expect(onChange).toHaveBeenLastCalledWith(icon("👩🏽‍🌾", "emoji"));
    fireEvent.click(screen.getByRole("button", { name: "Choose icon" }));
    expect(await screen.findByRole("tab", { name: "space.icon.emoji" })).toHaveAttribute("aria-selected", "true");
    fireEvent.click(screen.getByRole("button", { name: "space.icon.reset" }));
    expect(onChange).toHaveBeenLastCalledWith(undefined);
    await waitFor(() => expect(screen.queryByRole("combobox")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Choose icon" })).toHaveFocus();
    fireEvent.click(screen.getByRole("button", { name: "Choose icon" }));
    await screen.findByRole("combobox", { name: "space.icon.search-icons" });
    expect(screen.queryByRole("button", { name: "space.icon.reset" })).not.toBeInTheDocument();
  });

  it("keeps the selection unchanged when dismissed or when Enter completes IME composition", async () => {
    const onChange = vi.fn();
    render(<Harness initial={icon("leaf")} onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: "Choose icon" }));
    const search = await screen.findByRole("combobox", { name: "space.icon.search-icons" });
    expect(screen.getByRole("gridcell", { name: "Leaf" })).toHaveAttribute("aria-selected", "true");
    type(search, "book");
    fireEvent.keyDown(search, { key: "Enter", isComposing: true });
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.keyDown(search, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("combobox")).not.toBeInTheDocument());
    expect(onChange).not.toHaveBeenCalled();
  });

  it("shows empty search results without accepting invalid emoji text", async () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: "Choose icon" }));
    fireEvent.click(await screen.findByRole("tab", { name: "space.icon.emoji" }));
    const search = screen.getByRole("combobox", { name: "space.icon.search-emoji" });
    type(search, "🌱🌱");
    expect(screen.getByRole("status")).toHaveTextContent("space.icon.no-results");
    expect(screen.queryByRole("gridcell")).not.toBeInTheDocument();
    fireEvent.keyDown(search, { key: "Enter" });
    expect(onChange).not.toHaveBeenCalled();
  });

  it("disables editing while a form is saving", () => {
    render(<CustomIconPicker label="Choose icon" disabled onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Choose icon" })).toBeDisabled();
  });

  it("shows the default action only while a custom icon is set, on either tab", async () => {
    const onChange = vi.fn();
    const view = render(<CustomIconPicker label="Choose icon" onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: "Choose icon" }));
    await screen.findByRole("combobox", { name: "space.icon.search-icons" });
    expect(screen.queryByRole("button", { name: "space.icon.reset" })).not.toBeInTheDocument();

    view.rerender(<CustomIconPicker label="Choose icon" value={icon("leaf")} onChange={onChange} />);
    expect(screen.getByRole("button", { name: "space.icon.reset" })).toBeEnabled();
    fireEvent.click(screen.getByRole("tab", { name: "space.icon.emoji" }));
    expect(screen.getByRole("button", { name: "space.icon.reset" })).toBeEnabled();

    view.rerender(<CustomIconPicker label="Choose icon" onChange={onChange} />);
    expect(screen.queryByRole("button", { name: "space.icon.reset" })).not.toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("switches named content panels without selecting an icon", async () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: "Choose icon" }));
    const icons = await screen.findByRole("tab", { name: "space.icon.icons" });
    const emoji = screen.getByRole("tab", { name: "space.icon.emoji" });
    expect(screen.getByRole("tabpanel", { name: "space.icon.icons" })).toBeInTheDocument();
    icons.focus();
    fireEvent.keyDown(icons, { key: "ArrowRight" });
    await waitFor(() => expect(emoji).toHaveAttribute("aria-selected", "true"));
    expect(screen.getByRole("tabpanel", { name: "space.icon.emoji" })).toBeInTheDocument();
    expect(screen.queryByRole("tabpanel", { name: "space.icon.icons" })).not.toBeInTheDocument();
    expect(emoji).toHaveFocus();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("offers only emoji accepted by the shared Unicode catalog", () => {
    for (const [emoji] of SPACE_EMOJI) expect(FULLY_QUALIFIED_EMOJI).toContain(emoji);
  });
});
