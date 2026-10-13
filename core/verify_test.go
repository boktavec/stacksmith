package core

import (
	"bytes"
	"testing"
)

// assertSameFiles fails unless a and b hold the same paths, in the same
// order, with byte-identical content.
func assertSameFiles(t *testing.T, label string, a, b []File) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("%s: %d files vs %d files", label, len(a), len(b))
	}
	for i := range a {
		if a[i].Path != b[i].Path {
			t.Fatalf("%s: file %d path %q vs %q", label, i, a[i].Path, b[i].Path)
		}
		if !bytes.Equal(a[i].Content, b[i].Content) {
			t.Errorf("%s: %s content differs", label, a[i].Path)
		}
	}
}

// TestGenerateYAMLAndJSONOutputsAreIdentical compares the two formats'
// outputs directly (the golden test only proves parity transitively).
func TestGenerateYAMLAndJSONOutputsAreIdentical(t *testing.T) {
	yamlFiles := generateExample(t, FormatYAML)
	if len(yamlFiles) != len(generatedFileOrder) {
		t.Fatalf("got %d files; want %d", len(yamlFiles), len(generatedFileOrder))
	}
	assertSameFiles(t, "yaml vs json", yamlFiles, generateExample(t, FormatJSON))
}

// TestGenerateAllFilesAreDeterministic repeats Generate on one input and
// requires every one of the 8 files to be identical each time.
func TestGenerateAllFilesAreDeterministic(t *testing.T) {
	for _, format := range []Format{FormatYAML, FormatJSON} {
		first := generateExample(t, format)
		for run := 2; run <= 5; run++ {
			assertSameFiles(t, "repeat run", first, generateExample(t, format))
		}
	}
}
