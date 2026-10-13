package core

// PlatformStandards is the platform-owned document selecting the approved
// module, account/region, allowed environments, and fixed policy values.
// See docs/stacksmith-core-plan.md section 4 for the agreed input contract.
type PlatformStandards struct {
	APIVersion string        `yaml:"apiVersion" json:"apiVersion"`
	Kind       string        `yaml:"kind" json:"kind"`
	Spec       StandardsSpec `yaml:"spec" json:"spec"`
}

// StandardsSpec holds the platform's controlled values. There are no
// per-environment overrides in v0.1: environments share identical rules.
type StandardsSpec struct {
	AWS          AWSStandards    `yaml:"aws" json:"aws"`
	Environments []string        `yaml:"environments" json:"environments"`
	Bucket       BucketStandards `yaml:"bucket" json:"bucket"`
}

// AWSStandards fixes the one account and region v0.1 deploys into.
type AWSStandards struct {
	AccountID string `yaml:"accountId" json:"accountId"`
	Region    string `yaml:"region" json:"region"`
}

// BucketStandards selects the approved module and its platform-controlled
// values. bucketNamePrefix is optional and defaults to empty.
type BucketStandards struct {
	Module                          string `yaml:"module" json:"module"`
	BucketNamePrefix                string `yaml:"bucketNamePrefix" json:"bucketNamePrefix"`
	NoncurrentVersionExpirationDays int    `yaml:"noncurrentVersionExpirationDays" json:"noncurrentVersionExpirationDays"`
}
