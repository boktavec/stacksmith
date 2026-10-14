package core

// PlatformStandards is the platform-owned document fixing the account, region
// and allowed environments. See docs/spec/features/blueprint-catalog/SPEC.md
// section 4.
type PlatformStandards struct {
	APIVersion string        `yaml:"apiVersion" json:"apiVersion"`
	Kind       string        `yaml:"kind" json:"kind"`
	Spec       StandardsSpec `yaml:"spec" json:"spec"`
}

// StandardsSpec holds the platform's controlled values. Environments share
// identical rules; there are no per-environment overrides.
type StandardsSpec struct {
	AWS          AWSStandards `yaml:"aws" json:"aws"`
	Environments []string     `yaml:"environments" json:"environments"`
}

// AWSStandards fixes the one account and region StackSmith deploys into.
type AWSStandards struct {
	AccountID string `yaml:"accountId" json:"accountId"`
	Region    string `yaml:"region" json:"region"`
}
