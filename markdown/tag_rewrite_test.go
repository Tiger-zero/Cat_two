package markdown

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureTagPreservesMarkdown(t *testing.T) {
	svc := NewService(WithTagExtension())
	const tag = "MCP连接"
	for _, tc := range []struct {
		name    string
		content string
		want    string
	}{
		{"empty", "", "#MCP连接"},
		{"append after a titled note", "# 标题\n\n正文", "# 标题\n\n正文\n\n#MCP连接"},
		{"existing exact tag", "正文 #MCP连接", "正文 #MCP连接"},
		{"descendant does not replace exact tag", "正文 #MCP连接/子项", "正文 #MCP连接/子项\n\n#MCP连接"},
		{"literal uses", "`#MCP连接` [#MCP连接](https://example.com)", "`#MCP连接` [#MCP连接](https://example.com)\n\n#MCP连接"},
		{"open fence", "```md\n#MCP连接", "#MCP连接\n\n```md\n#MCP连接"},
		{"open fence after a title", "# 标题 ###\r\n```md\r\n文本", "# 标题 ###\r\n\n#MCP连接\n\n```md\r\n文本"},
		{"open HTML after a Setext title", "标题\n第二行\n===\n<pre>\n文本", "标题\n第二行\n===\n\n#MCP连接\n\n<pre>\n文本"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := svc.ExtractAll([]byte(tc.content))
			require.NoError(t, err)
			content, err := svc.EnsureTag(tc.content, tag)
			require.NoError(t, err)
			require.Equal(t, tc.want, content)
			after, err := svc.ExtractAll([]byte(content))
			require.NoError(t, err)
			require.Contains(t, after.Tags, tag)
			require.Equal(t, before.Property, after.Property)
			again, err := svc.EnsureTag(content, tag)
			require.NoError(t, err)
			require.Equal(t, content, again)
		})
	}
}

func TestRenameTagBranch(t *testing.T) {
	svc := NewService(WithTagExtension())
	content := "#year/08 #year/08/10 #year/08/d\u200day #year/080 #Year/08 #year\n\n" +
		"`#year/08/day` [#year/08](https://example.com) ![#year/08](image.png)"

	exact, err := svc.RenameTag([]byte(content), "year/08", "archive/august")
	require.NoError(t, err)
	require.Equal(t, "#archive/august #year/08/10 #year/08/d\u200day #year/080 #Year/08 #year\n\n"+
		"`#year/08/day` [#year/08](https://example.com) ![#year/08](image.png)", exact)

	renamed, err := svc.RenameTagBranch([]byte(content), "year/08", "archive/august")
	require.NoError(t, err)
	require.Equal(t, "#archive/august #archive/august/10 #archive/august/d\u200day #year/080 #Year/08 #year\n\n"+
		"`#year/08/day` [#year/08](https://example.com) ![#year/08](image.png)", renamed)
}

func TestRemoveTagBranch(t *testing.T) {
	svc := NewService(WithTagExtension())
	const content = "#project/now #project/now/one #project/now/two #project/now/three #project/later\n\n" +
		"Words #project/now kept [#project/now](example.png), ![#project/now](image.png) and `#project/now`.\n\n" +
		"#Project/now #project/nowish #project\n\n```md\n#project/now\n```"
	const expected = "#project/later\n\n" +
		"Words kept [#project/now](example.png), ![#project/now](image.png) and `#project/now`.\n\n" +
		"#Project/now #project/nowish #project\n\n```md\n#project/now\n```"
	removed, err := svc.RemoveTagBranch([]byte(content), "project/now")
	require.NoError(t, err)
	require.Equal(t, expected, removed)
	again, err := svc.RemoveTagBranch([]byte(removed), "project/now")
	require.NoError(t, err)
	require.Equal(t, expected, again)
}

func TestNormalizeTagWhitespace(t *testing.T) {
	svc := NewService(WithTagExtension(), WithMentionExtension())
	for _, tt := range []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "horizontal whitespace around separators",
			content: "#2027/ 08 #2027 /08 #2027\t/ \t08 /01 #工作\u3000/\u00a0日记。",
			want:    "#2027/08 #2027/08 #2027/08/01 #工作/日记。",
		},
		{
			name:    "descendant spelling and unrelated whitespace",
			content: "Before  #A\u200d / \u0301topic, #book / fiction  after.",
			want:    "Before  #A\u200d/\u0301topic, #book/fiction  after.",
		},
		{
			name:    "literal text in blockquotes tables and emphasis",
			content: "> **#year / 08**\n\n| #year/ 09 | notes |\n|---|---|\n| #year /10 | text |",
			want:    "> **#year/08**\n\n| #year/09 | notes |\n|---|---|\n| #year/10 | text |",
		},
		{
			name:    "code links images and escapes stay literal",
			content: "#year /08 `#year /08` [#year /08](url) ![#year /08](image.png)\n\n```md\n#year /08\n```\n\n\\#year /08 #year \\/ 08 &#35;year /08 #year &#47;08",
			want:    "#year/08 `#year /08` [#year /08](url) ![#year /08](image.png)\n\n```md\n#year /08\n```\n\n\\#year /08 #year \\/ 08 &#35;year /08 #year &#47;08",
		},
		{
			name:    "requires segments and does not cross lines",
			content: "#year / /08  #year / !  #year /  #month\n/08 #year/\n08 #tag  /",
			want:    "#year / /08  #year / !  #year /  #month\n/08 #year/\n08 #tag  /",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixed, err := svc.NormalizeTagWhitespace([]byte(tt.content))
			require.NoError(t, err)
			require.Equal(t, tt.want, fixed)
			fixedAgain, err := svc.NormalizeTagWhitespace([]byte(fixed))
			require.NoError(t, err)
			require.Equal(t, fixed, fixedAgain)
		})
	}
}

func TestSpacedTagNormalizationDoesNotAffectExtraction(t *testing.T) {
	svc := NewService(WithTagExtension())
	content := []byte("#2027/ 08 #2027 /09")
	tags, err := svc.ExtractTags(content)
	require.NoError(t, err)
	require.Equal(t, []string{"2027"}, tags)

	fixed, err := svc.NormalizeTagWhitespace(content)
	require.NoError(t, err)
	tags, err = svc.ExtractTags([]byte(fixed))
	require.NoError(t, err)
	require.Equal(t, []string{"2027", "2027/08", "2027/09"}, tags)
}
