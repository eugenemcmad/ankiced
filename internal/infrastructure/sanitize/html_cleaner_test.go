package sanitize

import "testing"

func applyDefault(t *testing.T, in string) string {
	t.Helper()
	out, err := HTMLCleanerTemplate{}.Apply(in)
	if err != nil {
		t.Fatalf("apply template: %v", err)
	}
	return out
}

func TestKeepBasicTags(t *testing.T) {
	in := `<div><b>bold</b><i>italic</i><br><u>u</u><span style="color:red">x</span></div>`
	got := applyDefault(t, in)
	want := `<div><b>bold</b><i>italic</i><br><u>u</u><span>x</span></div>`
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestKeepBasicTagsPreservesSelfClosingBR(t *testing.T) {
	got := applyDefault(t, `a<br/>b`)
	want := `a<br>b`
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestKeepBasicTagsPreservesSafeImageAttributes(t *testing.T) {
	in := `<p>x<img src="media/pic.png" alt="pic" title="title" width="10" height="20" onclick="evil()" onerror="bad()"></p>`
	got := applyDefault(t, in)
	want := `x<img src="media/pic.png" alt="pic" title="title" width="10" height="20">`
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestTemplateRegistryReturnsHTMLCleanerByDefault(t *testing.T) {
	registry := NewTemplateRegistry()
	template, err := registry.Default()
	if err != nil {
		t.Fatalf("default template: %v", err)
	}
	if template.ID() != "html_cleaner" {
		t.Fatalf("unexpected default template id %q", template.ID())
	}
	out, err := template.Apply(`<u>a</u><img src="x.png">`)
	if err != nil {
		t.Fatalf("apply template: %v", err)
	}
	if out != "<u>a</u><img src=\"x.png\">" {
		t.Fatalf("unexpected template output %q", out)
	}
}

func TestStripAllTagsReturnsPlainText(t *testing.T) {
	in := `<p>Hello <b>world</b><br>and <i>universe</i></p>`
	got := StripAllTags(in)
	want := "Hello worldand universe"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestStripAllTagsHandlesEmptyInput(t *testing.T) {
	if got := StripAllTags(""); got != "" {
		t.Fatalf("expected empty result, got %q", got)
	}
}

func TestKeepBasicTagsReencodesEntitiesInTextNodes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"ampersand", `<b>R&amp;D</b>`, `<b>R&amp;D</b>`},
		{"less-than-encoded", `<i>1 &lt; 2</i>`, `<i>1 &lt; 2</i>`},
		{"quotes-stay-text", `<u>"q"</u>`, `<u>&#34;q&#34;</u>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := applyDefault(t, tc.in)
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}
