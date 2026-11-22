import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { createRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EditorContent } from "@/components/MemoEditor/edit/components/EditorContent";
import { createInitialState, EditorProvider } from "@/components/MemoEditor/edit/state";
import { EditorToolbar } from "@/components/MemoEditor/edit/Toolbar/EditorToolbar";
import type { EditorController, EditorSelection } from "@/components/MemoEditor/edit/types/editorController";

const recordEditing = vi.hoisted(() => vi.fn());
vi.mock("@/lib/memo-editing-activity", () => ({ recordMemoEditingActivity: recordEditing }));
vi.mock("@/components/MemoEditor/edit/Toolbar/InsertMenu", () => ({ default: () => null }));
vi.mock("@/hooks/useUserQueries", () => ({
  useTagCounts: () => ({ data: { "work/planning": 2, home: 1 } }),
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
        onToggleFocusMode={vi.fn()}
        onInsertImages={vi.fn()}
      />
    </EditorProvider>,
  );
  const button = (name: string) => within(screen.getByRole("group", { name: "Formatting" })).getByRole("button", { name });
  return {
    container,
    editor: editorRef.current!,
    button,
    chooseFormat: async (name: string) => {
      fireEvent.click(button("More formatting"));
      const menu = within(await screen.findByRole("menu"));
      fireEvent.click(menu.getByRole("menuitem", { name }));
    },
  };
}

function select(editor: EditorController, anchor: EditorSelection["anchor"], focus = anchor) {
  act(() => {
    editor.focus();
    editor.setSelection({ anchor, focus });
  });
}

describe("composer formatting toolbar", () => {
  beforeEach(() => {
    recordEditing.mockClear();
    vi.spyOn(window, "scrollBy").mockImplementation(() => {});
  });
  afterEach(() => vi.restoreAllMocks());

  it.each([false, true])("reports document changes but leaves focus and draft restoration inactive (source: %s)", async (source) => {
    const { editor, button } = setup("restored draft", source);
    const text = screen.getByRole("textbox");
    select(editor, { block: 0, offset: 1 });
    fireEvent.keyDown(text, { key: "ArrowRight" });
    act(() => editor.setMarkdown("loaded"));
    expect(recordEditing).not.toHaveBeenCalled();

    select(editor, { block: 0, offset: 6 });
    fireEvent(text, new InputEvent("beforeinput", { bubbles: true, cancelable: true, inputType: "insertText", data: " note" }));
    await waitFor(() => expect(recordEditing).toHaveBeenCalledTimes(1));

    select(editor, { block: 0, offset: 0 }, { block: 0, offset: 6 });
    fireEvent.click(button("Bold"));
    await waitFor(() => expect(recordEditing).toHaveBeenCalledTimes(2));
  });

  it("ignores background image insertion and counts typing batched with a later upload", async () => {
    const { editor } = setup("draft");
    const text = screen.getByRole("textbox");
    const insertUploadedImage = (id: string) => {
      editor.createUploadAnchor({
        id,
        status: "uploading",
        completed: 0,
        total: 1,
        message: "Uploading",
        retryLabel: "Retry",
        keepLabel: "Cancel",
      });
      editor.resolveUploadAnchor(id, `![image](/file/attachments/${id})`);
    };

    await act(async () => {
      insertUploadedImage("first");
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    });
    expect(editor.getMarkdown()).toContain("/file/attachments/first");
    expect(recordEditing).not.toHaveBeenCalled();

    select(editor, { block: 0, offset: 5 });
    act(() => {
      fireEvent(text, new InputEvent("beforeinput", { bubbles: true, cancelable: true, inputType: "insertText", data: " edit" }));
      insertUploadedImage("second");
    });
    await waitFor(() => expect(recordEditing).toHaveBeenCalledOnce());
    expect(editor.getMarkdown()).toContain("draft edit");
    expect(editor.getMarkdown()).toContain("/file/attachments/second");
  });

  it.each([false, true])("starts and completes consecutive tags within prose (source: %s)", async (isSourceMode) => {
    const { editor } = setup("Keepword safe.", isSourceMode);
    const tag = screen.getByRole("button", { name: "Insert tag" });
    select(editor, { block: 0, offset: 1 }, { block: 0, offset: 4 });

    expect(fireEvent.mouseDown(tag)).toBe(false);
    fireEvent.click(tag);
    expect(editor.getSelection()?.focus.offset).toBe(6);
    expect(editor.hasFocus()).toBe(true);
    fireEvent.click(await screen.findByRole("option", { name: "#work/planning" }));
    expect(editor.getMarkdown()).toBe("Keep #work/planning word safe.");

    fireEvent.click(tag);
    fireEvent.click(await screen.findByRole("option", { name: "#home" }));
    expect(editor.getMarkdown()).toBe("Keep #work/planning #home word safe.");

    fireEvent.click(tag);
    fireEvent.click(await screen.findByRole("option", { name: "#work/planning" }));
    expect(editor.getMarkdown()).toBe("Keep #work/planning #home #work/planning word safe.");
    expect(editor.getTags()).toEqual(["work/planning", "home"]);
  });

  it.each([false, true])("opens existing tag suggestions in a new memo (source: %s)", async (isSourceMode) => {
    const { editor } = setup("", isSourceMode);
    const tag = screen.getByRole("button", { name: "Insert tag" });

    fireEvent.click(tag);
    fireEvent.click(await screen.findByRole("option", { name: "#work/planning" }));
    expect(editor.getMarkdown().trimEnd()).toBe("#work/planning");
  });

  it.each([false, true])("accepts a new nested tag typed before existing text (source: %s)", (isSourceMode) => {
    const { editor } = setup("Existing text", isSourceMode);
    select(editor, { block: 0, offset: 0 });
    fireEvent.click(screen.getByRole("button", { name: "Insert tag" }));
    fireEvent(
      screen.getByRole("textbox"),
      new InputEvent("beforeinput", { bubbles: true, cancelable: true, inputType: "insertText", data: "随手记/日常" }),
    );

    expect(editor.getMarkdown()).toBe("#随手记/日常 Existing text");
    expect(editor.getTags()).toEqual(["随手记/日常"]);
  });

  it.each([
    { markdown: "Before `code` after", offset: 9, source: false },
    { markdown: "Before `code` after", offset: 10, source: true },
    { markdown: "Before [link](https://example.com) after", offset: 9, source: false },
    { markdown: "Before [link](https://example.com) after", offset: 22, source: true },
    { markdown: "```\ncode\n```", offset: 2, source: false },
    { markdown: "```\ncode\n```", offset: 6, source: true },
  ])("keeps literal Markdown intact at the caret (source: $source, input: $markdown)", ({ markdown, offset, source }) => {
    const { editor } = setup(markdown, source);
    select(editor, { block: 0, offset });

    fireEvent.click(screen.getByRole("button", { name: "Insert tag" }));
    expect(editor.getMarkdown()).toBe(markdown);
    expect(screen.queryByRole("listbox")).toBeNull();
  });

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

  it.each([false, true])("formats selected text as italic and restores editor focus (source: %s)", (isSourceMode) => {
    const { editor, button } = setup("Keep word safe.", isSourceMode);
    select(editor, { block: 0, offset: 5 }, { block: 0, offset: 9 });
    const italic = button("Italic");

    expect(fireEvent.mouseDown(italic)).toBe(false);
    fireEvent.click(italic);
    expect(editor.getMarkdown()).toMatch(/^Keep ([_*])word\1 safe\.$/);
    expect(editor.hasFocus()).toBe(true);
    if (!isSourceMode) expect(italic).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(italic);
    expect(editor.getMarkdown()).toBe("Keep word safe.");
  });

  describe.each([false, true])("extended menu (source: %s)", (isSourceMode) => {
    it.each([
      { name: "Strikethrough", markdown: "Keep ~~word~~ safe." },
      { name: "Inline code", markdown: "Keep `word` safe." },
    ])("applies $name to the selected text", async ({ name, markdown }) => {
      const { editor, chooseFormat } = setup("Keep word safe.", isSourceMode);
      select(editor, { block: 0, offset: 5 }, { block: 0, offset: 9 });

      await chooseFormat(name);

      await waitFor(() => {
        expect(editor.getMarkdown()).toBe(markdown);
        const focused = document.activeElement;
        expect(editor.hasFocus(), `Focus landed on ${focused?.tagName} ${focused?.getAttribute("aria-label") ?? ""}`).toBe(true);
      });
    });

    it.each([
      { name: "Heading 2", first: "## One", second: "## Two" },
      { name: "Quote", first: "> One", second: "> Two" },
      { name: "To-do list", first: "- [ ] One", second: "- [ ] Two" },
    ])("applies $name across two selected paragraphs", async ({ name, first, second }) => {
      const { editor, chooseFormat } = setup(isSourceMode ? "Intro\n\nOne\nTwo\n\nOutro" : "Intro\n\nOne\n\nTwo\n\nOutro", isSourceMode);
      if (isSourceMode) {
        select(editor, { block: 0, offset: 7 }, { block: 0, offset: 14 });
      } else {
        select(editor, { block: 1, offset: 0 }, { block: 2, offset: 3 });
      }

      await chooseFormat(name);

      await waitFor(() => {
        const lines = editor
          .getMarkdown()
          .split("\n")
          .filter((line) => line.trim() && line.trim() !== ">");
        expect(lines).toEqual(["Intro", first, second, "Outro"]);
      });
    });

    it("converts a heading to a plain paragraph in the middle of the memo", async () => {
      const { editor, chooseFormat } = setup("Lead\n\n## Topic\n\nTail", isSourceMode);
      select(editor, isSourceMode ? { block: 0, offset: 10 } : { block: 1, offset: 1 });

      await chooseFormat("Paragraph");

      await waitFor(() => expect(editor.getMarkdown()).toBe("Lead\n\nTopic\n\nTail"));
    });

    it.each([
      { name: "Table", markup: /\|\s*-+\s*\|/ },
      { name: "Divider", markup: /^---$/m },
    ])("inserts a $name without replacing the selected text or the next paragraph", async ({ name, markup }) => {
      const { editor, chooseFormat } = setup("Before\n\nAfter", isSourceMode);
      select(editor, { block: 0, offset: 1 }, { block: 0, offset: 5 });

      await chooseFormat(name);

      await waitFor(() => {
        const markdown = editor.getMarkdown();
        expect(markdown).toMatch(/^Before\n\n/);
        expect(markdown.trimEnd()).toMatch(/\n\nAfter$/);
        expect(markdown).toMatch(markup);
      });
    });

    it("validates web links and preserves the selection while the dialog has focus", async () => {
      const { editor, chooseFormat } = setup("Read this later.", isSourceMode);
      select(editor, { block: 0, offset: 9 }, { block: 0, offset: 5 });

      await chooseFormat("Web link");

      const dialog = await screen.findByRole("dialog", { name: "Add web link" });
      const address = within(dialog).getByRole("textbox", { name: "Link address" });
      const submit = within(dialog).getByRole("button", { name: "Add link" });
      fireEvent.change(address, { target: { value: "javascript:alert(1)" } });
      fireEvent.click(submit);
      expect(await within(dialog).findByText("Enter a valid link address.")).toBeInTheDocument();
      expect(editor.getMarkdown()).toBe("Read this later.");

      fireEvent.change(address, { target: { value: "https://example.com/read" } });
      fireEvent.click(submit);
      await waitFor(() => expect(editor.getMarkdown()).toBe("Read [this](https://example.com/read) later."));
      await waitFor(() => expect(dialog).not.toBeInTheDocument());
      await waitFor(() => expect(editor.hasFocus()).toBe(true));
    });
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
