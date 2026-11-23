package markdown

import (
	"bytes"
	"cmp"
	"slices"
	"strings"

	gast "github.com/yuin/goldmark/ast"

	mast "github.com/usememos/memos/markdown/ast"
	"github.com/usememos/memos/markdown/extensions"
	"github.com/usememos/memos/markdown/parser"
)

// EnsureTag preserves the original Markdown and adds the tag as its own paragraph.
func (s *service) EnsureTag(content, tag string) (string, error) {
	paragraph := "#" + tag
	if content == "" {
		return paragraph, nil
	}
	withTag := content + "\n\n" + paragraph
	source := []byte(withTag)
	root, err := s.parse(source)
	if err != nil {
		return "", err
	}
	position := firstExactMemoTag(root, tag)
	switch {
	case position >= len(content):
		return withTag, nil
	case position >= 0:
		return content, nil
	}

	// An open code fence or HTML block can absorb a final paragraph. Move the
	// tag before the body, keeping any leading title as the first block.
	if end := memoTitleEnd(root, source[:len(content)]); end > 0 {
		return content[:end] + "\n" + paragraph + "\n\n" + content[end:], nil
	}
	return paragraph + "\n\n" + content, nil
}

func firstExactMemoTag(root gast.Node, path string) int {
	position := -1
	_ = gast.Walk(root, func(node gast.Node, entering bool) (gast.WalkStatus, error) {
		if tag, ok := asMemoTagNode(node); entering && ok && string(tag.Tag) == path {
			position = tag.Pos()
			return gast.WalkStop, nil
		}
		return gast.WalkContinue, nil
	})
	return position
}

func memoTitleEnd(root gast.Node, source []byte) int {
	heading, ok := root.FirstChild().(*gast.Heading)
	if !ok || heading.Level != 1 || heading.Lines().Len() == 0 {
		return 0
	}
	lines := heading.Lines()
	start := lines.At(0).Start
	lineStart := bytes.LastIndexByte(source[:start], '\n') + 1
	end := lines.At(lines.Len() - 1).Stop
	if end > 0 && source[end-1] != '\n' {
		end = nextMarkdownLine(source, end)
	}
	// For a Setext title, the heading segments omit its underline.
	if !bytes.HasPrefix(bytes.TrimLeft(source[lineStart:start], " \t"), []byte("#")) {
		end = nextMarkdownLine(source, end)
	}
	return end
}

func nextMarkdownLine(source []byte, offset int) int {
	if end := bytes.IndexByte(source[offset:], '\n'); end >= 0 {
		return offset + end + 1
	}
	return len(source)
}

// RenameTag renames exact occurrences of oldTag to newTag in content.
func (s *service) RenameTag(content []byte, oldTag, newTag string) (string, error) {
	root, err := s.parse(content)
	if err != nil {
		return "", err
	}
	replacement := "#" + newTag
	return rewriteMemoTags(content, root, func(tag *mast.TagNode) (string, bool) {
		return replacement, string(tag.Tag) == oldTag
	})
}

// RenameTagBranch renames a tag and its slash-delimited descendants in content.
func (s *service) RenameTagBranch(content []byte, oldTag, newTag string) (string, error) {
	if oldTag == newTag {
		return string(content), nil
	}
	root, err := s.parse(content)
	if err != nil {
		return "", err
	}
	prefix := oldTag + "/"
	depth := strings.Count(prefix, "/")
	replacement := "#" + newTag
	return rewriteMemoTags(content, root, func(tag *mast.TagNode) (string, bool) {
		value := string(tag.Tag)
		if value == oldTag {
			return replacement, true
		}
		if !strings.HasPrefix(value, prefix) {
			return "", false
		}
		// Preserve each descendant's spelling, including Unicode code points that
		// the scanner omits from its value. Slash separators are literal in both.
		suffix := string(tag.Source)
		for range depth {
			_, suffix, _ = strings.Cut(suffix, "/")
		}
		return replacement + "/" + suffix, true
	})
}

// RemoveTagBranch removes recognized tags in a slash-delimited branch.
func (s *service) RemoveTagBranch(content []byte, branch string) (string, error) {
	root, err := s.parse(content)
	if err != nil {
		return "", err
	}
	prefix := branch + "/"
	return rewriteMemoTags(content, root, func(tag *mast.TagNode) (string, bool) {
		value := string(tag.Tag)
		return "", value == branch || strings.HasPrefix(value, prefix)
	})
}

// NormalizeTagWhitespace repairs spaced hierarchy separators in recognized tags.
func (s *service) NormalizeTagWhitespace(content []byte) (string, error) {
	root, _ := s.parseSource(content, nil, extensions.AllowSpacedTagSeparators)
	return rewriteMemoTags(content, root, func(tag *mast.TagNode) (string, bool) {
		// The tolerant scanner only includes whitespace around a separator when
		// there is a valid segment on both sides of it.
		source := string(tag.Source)
		replacement := strings.Map(func(r rune) rune {
			if parser.IsTagHierarchyWhitespace(r) {
				return -1
			}
			return r
		}, source)
		return replacement, replacement != source
	})
}

func rewriteMemoTags(content []byte, root gast.Node, replace func(*mast.TagNode) (string, bool)) (string, error) {
	type sourceEdit struct {
		start int
		end   int
		text  string
	}
	var edits []sourceEdit
	if err := gast.Walk(root, func(node gast.Node, entering bool) (gast.WalkStatus, error) {
		if !entering {
			return gast.WalkContinue, nil
		}
		tag, ok := asMemoTagNode(node)
		if !ok || len(tag.Source) == 0 {
			return gast.WalkContinue, nil
		}
		if replacement, changed := replace(tag); changed {
			end := tag.Pos() + len(tag.Source)
			// Remove the following separator along with a deleted tag. Otherwise
			// a line of several removed tags can leave an indented code block.
			if replacement == "" && end < len(content) && (content[end] == ' ' || content[end] == '\t') {
				end++
			}
			edits = append(edits, sourceEdit{tag.Pos(), end, replacement})
		}
		return gast.WalkContinue, nil
	}); err != nil {
		return "", err
	}
	if len(edits) == 0 {
		return string(content), nil
	}

	slices.SortFunc(edits, func(left, right sourceEdit) int { return cmp.Compare(left.start, right.start) })
	var output strings.Builder
	output.Grow(len(content))
	cursor := 0
	for _, edit := range edits {
		output.Write(content[cursor:edit.start])
		output.WriteString(edit.text)
		cursor = edit.end
	}
	output.Write(content[cursor:])
	return output.String(), nil
}
