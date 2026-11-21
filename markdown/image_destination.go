package markdown

import (
	"errors"
	"slices"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type imageReferenceRanges map[*ast.Image]struct{ start, end int }

var imageReferenceRangesKey = parser.NewContextKey()

type imageReferenceParser struct {
	parser.InlineParser
	parser.CloseBlocker
}

func newMarkdownParser() parser.Parser {
	linkParser := parser.NewLinkParser()
	inline := parser.DefaultInlineParsers()
	for i, candidate := range inline {
		if candidate.Value == linkParser {
			inline[i].Value = &imageReferenceParser{linkParser, linkParser.(parser.CloseBlocker)}
			break
		}
	}
	return parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(inline...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
}

func (p *imageReferenceParser) Parse(parent ast.Node, reader text.Reader, ctx parser.Context) ast.Node {
	ranges, ok := ctx.Get(imageReferenceRangesKey).(imageReferenceRanges)
	if !ok {
		return p.InlineParser.Parse(parent, reader, ctx)
	}
	_, before := reader.Position()
	node := p.InlineParser.Parse(parent, reader, ctx)
	if image, ok := node.(*ast.Image); ok && image.Reference != nil {
		_, after := reader.Position()
		ranges[image] = struct{ start, end int }{before.Start + 1, after.Start}
	}
	return node
}

type imageDestinationEdit struct {
	start int
	end   int
	value string
}

// RewriteImageDestinations preserves source outside changed image destinations.
// If a reference definition also serves a regular link, only the image changes.
func (s *service) RewriteImageDestinations(content []byte, rewrite func(string) (string, error)) (string, error) {
	ranges := make(imageReferenceRanges)
	root, source := s.parseSource(content, ranges)
	links := make(map[*byte]bool)
	var images []*ast.Image
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch node := node.(type) {
			case *ast.Link:
				if len(node.Destination) > 0 {
					links[&node.Destination[0]] = true
				}
			case *ast.Image:
				if len(node.Destination) > 0 {
					images = append(images, node)
				}
			}
		}
		return ast.WalkContinue, nil
	})

	destinations := make(map[*byte]imageDestinationEdit)
	var edits []imageDestinationEdit
	for _, image := range images {
		destination := string(image.Destination)
		replacement, err := rewrite(destination)
		if err != nil {
			return "", err
		}
		if replacement == destination {
			continue
		}
		pointer := &image.Destination[0]
		if links[pointer] {
			span, ok := ranges[image]
			if !ok || span.start <= 0 || span.start > span.end || span.end > len(source) || source[span.start-1] != ']' {
				return "", errors.New("cannot locate Markdown image reference in source")
			}
			edits = append(edits, imageDestinationEdit{span.start, span.end, inlineImageDestination(replacement, image.Title)})
			continue
		}
		destinations[pointer] = imageDestinationEdit{end: len(destination), value: replacement}
	}
	// Goldmark stores parsed destinations as slices of its input. Locate each
	// edit by identity so identical text in code or unused definitions stays put.
	if len(destinations) > 0 {
		for start := range source {
			p := &source[start]
			if change, ok := destinations[p]; ok {
				edits = append(edits, imageDestinationEdit{start, start + change.end, change.value})
				delete(destinations, p)
			}
		}
		if len(destinations) > 0 {
			return "", errors.New("cannot locate Markdown image destination in source")
		}
	}
	if len(edits) == 0 {
		return string(content), nil
	}
	slices.SortFunc(edits, func(a, b imageDestinationEdit) int { return a.start - b.start })
	var output strings.Builder
	output.Grow(len(content))
	cursor := 0
	for _, change := range edits {
		if change.start < cursor || change.end > len(content) {
			return "", errors.New("overlapping Markdown image destination edits")
		}
		output.Write(content[cursor:change.start])
		output.WriteString(change.value)
		cursor = change.end
	}
	output.Write(content[cursor:])
	return output.String(), nil
}

func inlineImageDestination(destination string, title []byte) string {
	var output strings.Builder
	output.WriteString("(<")
	output.WriteString(destination)
	output.WriteByte('>')
	if len(title) > 0 {
		output.WriteString(` "`)
		for i := 0; i < len(title); i++ {
			if title[i] == '\\' && i+1 < len(title) && util.IsPunct(title[i+1]) {
				output.WriteByte(title[i])
				i++
			} else if title[i] == '"' {
				output.WriteByte('\\')
			}
			output.WriteByte(title[i])
		}
		output.WriteByte('"')
	}
	output.WriteByte(')')
	return output.String()
}
