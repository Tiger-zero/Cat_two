package markdown

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
