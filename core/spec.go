package core

// Spec is a developer-authored bucket specification: one file, one bucket.
// See docs/stacksmith-core-plan.md section 4 for the agreed input contract.
type Spec struct {
	APIVersion string       `yaml:"apiVersion" json:"apiVersion"`
	Kind       string       `yaml:"kind" json:"kind"`
	Metadata   SpecMetadata `yaml:"metadata" json:"metadata"`
	Spec       BucketSpec   `yaml:"spec" json:"spec"`
}

// SpecMetadata holds the developer's logical name for the bucket. It is not
// the final AWS bucket name.
type SpecMetadata struct {
	Name string `yaml:"name" json:"name"`
}

// BucketSpec is the developer-controlled portion of the contract: intent,
// not implementation. There are no fields here for security, encryption,
// retention, tags, module source, account, or region.
type BucketSpec struct {
	Environment string `yaml:"environment" json:"environment"`
	Owner       string `yaml:"owner" json:"owner"`
}
