package core

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const pinnedBlueprintsCommit = "ed819adb2c5c71790c41a0510f604b38f1c9a6ed"

func TestCatalogListsSnapshotBlueprintsInIDOrder(t *testing.T) {
	entries, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{"argocd", "bastion", "dns", "eks", "network", "rds", "s3-event-worker"}
	var gotIDs []string
	for _, entry := range entries {
		gotIDs = append(gotIDs, entry.ID)
		if entry.Summary == "" {
			t.Errorf("%s has an empty summary", entry.ID)
		}
		if entry.Supported != (entry.ID == "network") {
			t.Errorf("%s: Supported = %v", entry.ID, entry.Supported)
		}
		if entry.ID == "network" {
			const want = "VPC across N AZs (flow logs on, EKS-tagged subnets); publishes vpc/subnets to SSM."
			if entry.Summary != want {
				t.Errorf("network summary = %q", entry.Summary)
			}
		}
	}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("IDs = %v, want %v", gotIDs, wantIDs)
	}
}

// Keeps the supported table honest against the real snapshot: a refreshed
// catalog that moves a supported blueprint's template or module must fail here.
func TestSupportedBlueprintsMatchSnapshot(t *testing.T) {
	data, err := snapshotFS.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	file, err := decodeCatalog(data)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range supportedBlueprints {
		entry, ok := file.Blueprints[id]
		if !ok {
			t.Errorf("supported blueprint %q is not in the snapshot", id)
			continue
		}
		if entry.Template != want.template || entry.Module != want.module {
			t.Errorf("%s: snapshot template/module = %q/%q, table = %q/%q",
				id, entry.Template, entry.Module, want.template, want.module)
		}
	}
}

const validCatalog = `aws_provider_version: "~> 6.0"
blueprints:
  network:
    summary: "s"
    providers:
      - {name: helm, source: hashicorp/helm, version: "~> 3.0"}
`

func TestDecodeCatalog(t *testing.T) {
	if _, err := decodeCatalog([]byte(validCatalog)); err != nil {
		t.Fatalf("valid catalog rejected: %v", err)
	}
	for name, input := range map[string]string{
		"unknown top-level field": validCatalog + "extra: 1\n",
		"unknown entry field":     strings.Replace(validCatalog, "summary:", "bogus: 1\n    summary:", 1),
		"unknown provider field":  strings.Replace(validCatalog, "version: \"~> 3.0\"}", "version: \"~> 3.0\", extra: x}", 1),
		"duplicate key":           validCatalog + "aws_provider_version: \"~> 5.0\"\n",
		"duplicate blueprint ID":  validCatalog + "  network:\n    summary: \"again\"\n",
		"summary wrong type":      strings.Replace(validCatalog, `summary: "s"`, "summary: [x]", 1),
		"providers wrong type":    strings.Replace(validCatalog, "    providers:\n      - {name: helm, source: hashicorp/helm, version: \"~> 3.0\"}\n", "    providers: x\n", 1),
		"tier value wrong type":   strings.Replace(validCatalog, "    providers:", "    tiers:\n      dev:\n        size: [x]\n    providers:", 1),
		"wrong kind":              "- network\n- eks\n",
		"second document":         validCatalog + "---\n" + validCatalog,
		"empty blueprints":        "aws_provider_version: \"~> 6.0\"\nblueprints: {}\n",
		"missing blueprints":      "aws_provider_version: \"~> 6.0\"\n",
		"empty input":             "",
		"invalid YAML":            "blueprints: [unterminated\n",
		"invalid trailing YAML":   validCatalog + "---\n[unterminated\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeCatalog([]byte(input)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

var (
	commitLine = regexp.MustCompile(`^commit ([0-9a-f]{40})$`)
	sumLine    = regexp.MustCompile(`^([0-9a-f]{64})  (\S+)$`)
)

func TestManifestMatchesEmbeddedSnapshot(t *testing.T) {
	raw, err := snapshotFS.ReadFile("snapshot/foggy-blueprints/MANIFEST")
	if err != nil {
		t.Fatal(err)
	}
	var repository, commit string
	checksums := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if url, ok := strings.CutPrefix(line, "repository "); ok {
			if repository != "" {
				t.Fatal("duplicate repository line")
			}
			repository = url
		} else if m := commitLine.FindStringSubmatch(line); m != nil {
			if commit != "" {
				t.Fatal("duplicate commit line")
			}
			commit = m[1]
		} else if m := sumLine.FindStringSubmatch(line); m != nil {
			if _, dup := checksums[m[2]]; dup {
				t.Fatalf("duplicate checksum line for %s", m[2])
			}
			checksums[m[2]] = m[1]
		} else {
			t.Fatalf("malformed MANIFEST line %q", line)
		}
	}
	if repository != "https://github.com/FoggyOps-LLC/foggy-blueprints" {
		t.Errorf("repository = %q", repository)
	}
	if commit != pinnedBlueprintsCommit {
		t.Errorf("commit = %q, want %q", commit, pinnedBlueprintsCommit)
	}

	snapshotRoot, err := fs.Sub(snapshotFS, "snapshot/foggy-blueprints")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	err = fs.WalkDir(snapshotRoot, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path == "MANIFEST" {
			return err
		}
		seen++
		data, err := fs.ReadFile(snapshotRoot, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		if got := checksums[path]; got != hex.EncodeToString(sum[:]) {
			t.Errorf("%s: MANIFEST checksum %q, actual %x", path, got, sum)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != len(checksums) {
		t.Errorf("MANIFEST lists %d files, snapshot has %d", len(checksums), seen)
	}
}
