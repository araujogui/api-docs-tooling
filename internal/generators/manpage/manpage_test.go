package manpage

import (
	"testing"

	"github.com/nodejs/doc-kit/internal/doc"
	. "github.com/nodejs/doc-kit/internal/testutil"
	"github.com/nodejs/doc-kit/internal/unist"
)

func mockEntry(headingText, description string) *doc.Entry {
	return &doc.Entry{
		Heading: &unist.Node{Type: "heading", Data: &unist.Data{Text: headingText}},
		// The first child stands for the heading, which conversion drops
		Content: U("root", U("heading"), Paragraph(Text(description))),
	}
}

func TestConvertNodeToMandoc(t *testing.T) {
	root := U("root", Heading("Main Title", 1), Paragraph(Text("Introductory text.")))

	if got := ConvertNodeToMandoc(root, false); got != ".Sh Main Title\nIntroductory text." {
		t.Errorf("got %q", got)
	}

	cases := []struct {
		node *unist.Node
		want string
	}{
		{Heading("Heading", 2), ".Sh Heading"},
		{Text("Some text"), "Some text"},
		{Text(`Text with a backslash: \\`), `Text with a backslash: \\\\`},
		{U("list", U("listItem", Text("Item 1")), U("listItem", Text("Item 2"))), ".Bl -bullet\n.It\nItem 1\n.It\nItem 2\n.El"},
		{U("code", "const a = 1;"), ".Bd -literal\nconst a = 1;\n.Ed"},
		{U("inlineCode", "inline code"), `\fBinline code\fR`},
		{U("emphasis", Text("emphasized text")), `\fIemphasized text\fR`},
		{U("blockquote", Paragraph(Text("This is a quote."))), ""},
		{U("paragraph"), ""},
		{U("list"), ".Bl -bullet\n\n.El"},
		{U("strong", Text("strongly emphasized text")), `\fBstrongly emphasized text\fR`},
		{U("typeAnnotation", "string"), `\fB{string}\fR`},
	}

	for _, c := range cases {
		if got := ConvertNodeToMandoc(c.node, false); got != c.want {
			t.Errorf("%s: got %q, want %q", c.node.Type, got, c.want)
		}
	}
}

func TestFlagValueToMandoc(t *testing.T) {
	cases := map[string]string{
		"-o value":                          " Ar value",
		"--flag=value":                      " Ns = Ns Ar value",
		"-n":                                "",
		"-f value1,value2":                  " Ar value1,value2",
		"--multi-flag=value1,value2,value3": " Ns = Ns Ar value1,value2,value3",
		"--flag[=optional value]":           " Ns = Ns Ar [optional value]",
	}

	for in, want := range cases {
		if got := FlagValueToMandoc(in); got != want {
			t.Errorf("FlagValueToMandoc(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConvertOptionToMandoc(t *testing.T) {
	cases := []struct{ heading, description, want string }{
		{"`-a`, `-b=value`", "Description of the options.", ".It Fl a , Fl b Ns = Ns Ar value\nDescription of the options.\n.\n"},
		{"`-a`", "Description of the option without a value.", ".It Fl a\nDescription of the option without a value.\n.\n"},
		{"`-x`, `-y`, `-z=value`", "Description of multiple options.", ".It Fl x , Fl y , Fl z Ns = Ns Ar value\nDescription of multiple options.\n.\n"},
		{"`-d`, `--option=value with spaces`", "Description of special options.", ".It Fl d , Fl -option Ns = Ns Ar value with spaces\nDescription of special options.\n.\n"},
	}

	for _, c := range cases {
		if got := ConvertOptionToMandoc(mockEntry(c.heading, c.description)); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}

func TestConvertEnvVarToMandoc(t *testing.T) {
	cases := []struct{ heading, description, want string }{
		{"`MY_VAR=some_value`", "Description of the environment variable.", ".It Ev MY_VAR Ar some_value\nDescription of the environment variable.\n.\n"},
		{"`MY_VAR=`", "Description of the environment variable without a value.", ".It Ev MY_VAR\nDescription of the environment variable without a value.\n.\n"},
		{"`SPECIAL_VAR=special value!`", "Description of special environment variable.", ".It Ev SPECIAL_VAR Ar special value!\nDescription of special environment variable.\n.\n"},
	}

	for _, c := range cases {
		if got := ConvertEnvVarToMandoc(mockEntry(c.heading, c.description)); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}
