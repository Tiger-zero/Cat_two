import { fireEvent, render, screen } from "@testing-library/react";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { createInitialState, EditorProvider, useEditorContext, useEditorSelector } from "@/components/MemoEditor/edit/state";
import { EditorToolbar } from "@/components/MemoEditor/edit/Toolbar/EditorToolbar";
import SpaceMenu from "@/components/MemoEditor/edit/Toolbar/SpaceMenu";

const spacesQuery = vi.hoisted(() => ({
  data: [
    { name: "spaces/work", title: "Work", icon: { value: { case: "emoji", value: "💎" } } },
    { name: "spaces/reading", title: "Reading" },
  ] as { name: string; title: string; icon?: unknown }[],
  isPending: false,
  isError: false,
  refetch: vi.fn(),
}));
vi.mock("@/components/MemoEditor/edit/Toolbar/InsertMenu", () => ({ default: () => null }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => ({ name: "users/test" }) }));
vi.mock("@/hooks/useSpaceQueries", () => ({ useSpaces: () => spacesQuery }));

// Echo interpolation so tests can see which Space a string names.
vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string, options?: { space?: string }) => (options?.space ? `${key}(${options.space})` : key),
}));

// Base UI menus reach for layout/pointer APIs jsdom doesn't implement.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
  Element.prototype.hasPointerCapture = vi.fn(() => false);
  Element.prototype.setPointerCapture = vi.fn();
  Element.prototype.releasePointerCapture = vi.fn();
});

const openMenu = async () => {
  fireEvent.click(screen.getByRole("button"));
  await screen.findByRole("menu");
};

beforeEach(() => {
  vi.clearAllMocks();
  spacesQuery.isPending = false;
  spacesQuery.isError = false;
  spacesQuery.data = [
    { name: "spaces/work", title: "Work", icon: { value: { case: "emoji", value: "💎" } } },
    { name: "spaces/reading", title: "Reading" },
  ];
});

describe("SpaceMenu", () => {
  it("organizes the memo without offering audience choices", async () => {
    const onChange = vi.fn();
    render(<SpaceMenu onChange={onChange} />);
    expect(screen.getByRole("button")).toHaveAccessibleName("space.add-to");
    await openMenu();

    expect(screen.queryByRole("menuitemradio")).not.toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Reading" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("menuitem", { name: "Work" }));

    expect(onChange).toHaveBeenCalledWith("spaces/work");
  });

  it.each([
    ["spaces/work", "Work"],
    ["spaces/gone", "gone"],
  ])("can clear the selected Space %s", async (space, label) => {
    const onChange = vi.fn();
    render(<SpaceMenu space={space} onChange={onChange} />);
    expect(screen.getByRole("button")).toHaveAccessibleName(label);
    await openMenu();

    fireEvent.click(screen.getByRole("menuitem", { name: `space.remove-from(${label})` }));

    expect(onChange).toHaveBeenCalledWith(undefined);
  });

  it("disambiguates duplicate Space titles", async () => {
    spacesQuery.data = [
      { name: "spaces/a", title: "Twin" },
      { name: "spaces/b", title: "Twin" },
    ];
    render(<SpaceMenu onChange={vi.fn()} />);
    await openMenu();
    expect(screen.getByRole("menuitem", { name: "Twin (a)" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Twin (b)" })).toBeInTheDocument();
  });

  it("hides the chooser when there are no Spaces", () => {
    spacesQuery.data = [];
    render(<SpaceMenu onChange={vi.fn()} />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("allows retrying a failed Space query", async () => {
    spacesQuery.data = [];
    spacesQuery.isError = true;
    render(<SpaceMenu onChange={vi.fn()} />);
    await openMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: /space\.load-error/ }));
    expect(spacesQuery.refetch).toHaveBeenCalledOnce();
  });
});

it("removing a memo from its Space keeps its content", async () => {
  let getState: ReturnType<typeof useEditorContext>["getState"];
  function Toolbar() {
    getState = useEditorContext().getState;
    const space = useEditorSelector((state) => state.metadata.space);
    return (
      <EditorToolbar
        space={space}
        canChooseSpace
        onSave={vi.fn()}
        onAudioRecorderClick={vi.fn()}
        sourceMode={{ isSourceMode: false, onToggle: vi.fn() }}
        onInsertImages={vi.fn()}
      />
    );
  }
  const initial = createInitialState();
  initial.content = "Keep this draft";
  initial.metadata = { ...initial.metadata, space: "spaces/work" };
  render(
    <EditorProvider initialEditorState={initial}>
      <Toolbar />
    </EditorProvider>,
  );

  fireEvent.click(screen.getByRole("button", { name: "Work" }));
  await screen.findByRole("menu");
  fireEvent.click(screen.getByRole("menuitem", { name: "space.remove-from(Work)" }));

  expect(getState!().metadata.space).toBeUndefined();
  expect(getState!().content).toBe("Keep this draft");
});
