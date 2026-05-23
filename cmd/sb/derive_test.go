package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetectShenRuntime covers the textual scan that decides whether
// `tools.shen_runtime` gets populated in the discharge report. The
// detection must (a) ignore specs with no `:runtime-via` marker,
// (b) recognise the Shen-block-comment form documented in
// docs/RUNTIME-VIA.md, (c) tolerate missing files (the report is
// best-effort), and (d) survive a nil config.
func TestDetectShenRuntime(t *testing.T) {
	dir := t.TempDir()

	plain := filepath.Join(dir, "plain.shen")
	if err := os.WriteFile(plain, []byte(`(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)`), 0o644); err != nil {
		t.Fatal(err)
	}

	annotated := filepath.Join(dir, "annotated.shen")
	if err := os.WriteFile(annotated, []byte(`(datatype query-text
  X : string;
  (> (length X) 0) : verified; \* :runtime-via shenEval *\
  ==============
  X : query-text;)`), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		cfg  *Config
		want string
	}{
		{"nil cfg", nil, ""},
		{"no derive specs", &Config{}, ""},
		{
			name: "all plain specs",
			cfg: &Config{DeriveSpecs: []DeriveSpec{{Path: plain}}},
			want: "",
		},
		{
			name: "annotated spec returns shen-sbcl",
			cfg: &Config{DeriveSpecs: []DeriveSpec{{Path: annotated}}},
			want: "shen-sbcl",
		},
		{
			name: "mixed: one annotated, one plain → shen-sbcl",
			cfg: &Config{DeriveSpecs: []DeriveSpec{
				{Path: plain},
				{Path: annotated},
			}},
			want: "shen-sbcl",
		},
		{
			name: "missing file is ignored",
			cfg: &Config{DeriveSpecs: []DeriveSpec{
				{Path: filepath.Join(dir, "does-not-exist.shen")},
			}},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := detectShenRuntime(tc.cfg)
			if got != tc.want {
				t.Errorf("detectShenRuntime: got %q, want %q", got, tc.want)
			}
		})
	}
}
