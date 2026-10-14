package core

// Spec is a developer-authored Blueprint document: one file, one blueprint
// instance. See docs/spec/features/blueprint-catalog/SPEC.md section 4.
type Spec struct {
	APIVersion string        `yaml:"apiVersion" json:"apiVersion"`
	Kind       string        `yaml:"kind" json:"kind"`
	Metadata   SpecMetadata  `yaml:"metadata" json:"metadata"`
	Spec       BlueprintSpec `yaml:"spec" json:"spec"`
}

// SpecMetadata holds the developer's name for this blueprint instance.
type SpecMetadata struct {
	Name string `yaml:"name" json:"name"`
}

// BlueprintSpec is the developer-controlled portion of the contract: which
// catalog blueprint, in which environment, at which tier. Parameters holds the
// developer's overrides of the tier preset; Validate checks them against the
// platform standards. A nil map and an empty map differ under
// reflect.DeepEqual, so callers should test len(Parameters), not nil-ness.
type BlueprintSpec struct {
	Blueprint   string            `yaml:"blueprint" json:"blueprint"`
	Environment string            `yaml:"environment" json:"environment"`
	Tier        string            `yaml:"tier" json:"tier"`
	Parameters  map[string]string `yaml:"parameters" json:"parameters"`
}
