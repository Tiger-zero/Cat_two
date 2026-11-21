import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { createRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EditorContent } from "@/components/MemoEditor/edit/components/EditorContent";
import { createInitialState, EditorProvider } from "@/components/MemoEditor/edit/state";
import { EditorToolbar } from "@/components/MemoEditor/edit/Toolbar/EditorToolbar";
import type { EditorController } from "@/components/MemoEditor/edit/types/editorController";

vi.mock("@/components/MemoEditor/edit/Toolbar/InsertMenu", () => ({ default: () => null }));
vi.mock("@/hooks/useUserQueries", () => ({
  useTagCounts: () => ({ data: {} }),
  useUsersByUsernames: () => ({ data: undefined }),
  userDetailQueryOptions: () => ({}),
}));
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ userTagsSetting: undefined }) }));

function setup(markdown: string, isSourceMode = false) {
  const editorRef = createRef<EditorController>();
  const sourceMode = { isSourceMode, onToggle: vi.fn() };
  const state = createInitialState();
  state.content = markdown;
  const { container } = render(
    <EditorProvider initialEditorState={state}>
      <EditorContent ref={editorRef} placeholder="Memo" sourceMode={sourceMode} onSubmit={vi.fn()} onFiles={vi.fn()} />
      <EditorToolbar
        editorRef={editorRef}
        onSave={vi.fn()}
        sourceMode={sourceMode}
        onAudioRecorderClick={vi.fn()}
        onInsertImages={vi.fn()}
      />
    </EditorProvider>,
  );
  return {
    container,
    editor: editorRef.current!,
    button: (name: string) => within(screen.getByRole("group", { name: "Formatting" })).getByRole("button", { name }),
  };
}

describe("composer formatting toolbar", () => {
  beforeEach(() => vi.spyOn(window, "scrollBy").mockImplementation(() => {}));
  afterEach(() => vi.restoreAllMocks());

  it.each([false, true])("toggles bold across a backwards selection and restores editor focus (source: %s)", (isSourceMode) => {
    const { container, editor, button } = setup("Keep bold word.", isSourceMode);
    const bold = button("Bold");
    act(() => {
      editor.focus();
      editor.setSelection({ anchor: { block: 0, offset: 9 }, focus: { block: 0, offset: 5 } });
    });

    expect(fireEvent.mouseDown(bold)).toBe(false);
    fireEvent.click(bold);
    expect(editor.getMarkdown()).toBe("Keep **bold** word.");
    expect(editor.hasFocus()).toBe(true);
    if (isSourceMode) {
      expect(bold).not.toHaveAttribute("aria-pressed");
    } else {
      expect(bold).toHaveAttribute("aria-pressed", "true");
      expect(container.querySelector("strong")).toHaveTextContent("bold");
    }

    act(() => bold.focus());
    fireEvent.click(bold);
    expect(editor.getMarkdown()).toBe("Keep bold word.");
    expect(editor.hasFocus()).toBe(true);
  });

  it("updates the rich formatting button when the caret moves and when the shortcut is used", () => {
    const { container, editor, button } = setup("Keep **bold** word.");
    const bold = button("Bold");
    expect(bold).toHaveAttribute("aria-pressed", "false");

    act(() => {
      editor.focus();
      editor.setSelection({ anchor: { block: 0, offset: 7 }, focus: { block: 0, offset: 7 } });
    });
    expect(bold).toHaveAttribute("aria-pressed", "true");

    act(() => editor.setSelection({ anchor: { block: 0, offset: 12 }, focus: { block: 0, offset: 12 } }));
    expect(bold).toHaveAttribute("aria-pressed", "false");
    fireEvent.keyDown(container.querySelector(".memo-editor-root")!, { key: "b", ctrlKey: true });
    expect(bold).toHaveAttribute("aria-pressed", "true");
    expect(editor.getMarkdown()).toBe("Keep **bold** word.");
  });

  it("reports list formatting as active only when applied to the whole rich text selection", () => {
    const { editor, button } = setup("One\n\n- Two");
    const bullets = button("Bullet list");
    act(() => {
      editor.focus();
      editor.setSelection({ anchor: { block: 0, offset: 0 }, focus: { block: 1, offset: 3 } });
    });

    expect(bullets).toHaveAttribute("aria-pressed", "false");
    fireEvent.click(bullets);
    expect(editor.getMarkdown()).toBe("- One\n- Two");
    expect(bullets).toHaveAttribute("aria-pressed", "true");
  });

  it.each([
    { isSourceMode: false, value: "Intro\n\nOne\n\nTwo\n\nOutro", anchor: { block: 1, offset: 0 }, focus: { block: 2, offset: 3 } },
    { isSourceMode: true, value: "Intro\n\nOne\nTwo\n\nOutro", anchor: { block: 0, offset: 7 }, focus: { block: 0, offset: 14 } },
  ])("switches selected paragraphs between numbered and bullet lists (source: $isSourceMode)", ({ isSourceMode, value, anchor, focus }) => {
    const { editor, button } = setup(value, isSourceMode);
    act(() => {
      editor.focus();
      editor.setSelection({ anchor, focus });
    });

    fireEvent.click(button("Numbered list"));
    expect(editor.getMarkdown()).toBe("Intro\n\n1. One\n2. Two\n\nOutro");
    fireEvent.click(button("Bullet list"));
    expect(editor.getMarkdown()).toBe("Intro\n\n- One\n- Two\n\nOutro");
  });

  it.each([false, true])("creates a fenced code block using the current block (source: %s)", (isSourceMode) => {
    const { editor, button } = setup("example()", isSourceMode);
    act(() => {
      editor.focus();
      editor.setSelection({ anchor: { block: 0, offset: 5 }, focus: { block: 0, offset: 5 } });
    });

    fireEvent.click(button("Code block"));
    expect(editor.getMarkdown().trimEnd()).toBe("```\nexample()\n```");
  });
});
