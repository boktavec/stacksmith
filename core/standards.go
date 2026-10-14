package core

// PlatformStandards is the platform-owned document fixing the account, region
// and allowed environments. See docs/spec/features/blueprint-catalog/SPEC.md
// section 4.
type PlatformStandards struct {
	APIVersion string        `yaml:"apiVersion" json:"apiVersion"`
	Kind       string        `yaml:"kind" json:"kind"`
	Spec       StandardsSpec `yaml:"spec" json:"spec"`
}

// StandardsSpec holds the platform's controlled values. BlueprintOverrides
// says which parameters a developer may override; absent means none.
type StandardsSpec struct {
	AWS                AWSStandards                  `yaml:"aws" json:"aws"`
	Environments       []string                      `yaml:"environments" json:"environments"`
	BlueprintOverrides map[string]BlueprintOverrides `yaml:"blueprintOverrides" json:"blueprintOverrides"`
}

// BlueprintOverrides holds the override policy for one blueprint, per tier.
type BlueprintOverrides struct {
	Tiers map[string]TierOverrides `yaml:"tiers" json:"tiers"`
}

// TierOverrides holds the policy for each overridable parameter of one tier.
// A parameter with no entry cannot be overridden.
type TierOverrides struct {
	Parameters map[string]ParameterPolicy `yaml:"parameters" json:"parameters"`
}

// ParameterPolicy constrains the values a developer may set. The zero value
// (written {} in a document) permits any canonical value within the module's
// intrinsic bounds.
//
// Minimum and Maximum are pointers so "not set" (nil) differs from 0. Allowed
// is a pointer to a slice so an explicit empty list ([]), which is an error,
// differs from an absent one (nil).
type ParameterPolicy struct {
	Minimum *int      `yaml:"minimum" json:"minimum"`
	Maximum *int      `yaml:"maximum" json:"maximum"`
	Allowed *[]string `yaml:"allowed" json:"allowed"`
}

// AWSStandards fixes the one account and region StackSmith deploys into.
type AWSStandards struct {
	AccountID string `yaml:"accountId" json:"accountId"`
	Region    string `yaml:"region" json:"region"`
}
