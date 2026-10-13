package core

// resolved is the generator's internal view of one validated bucket: the
// developer's intent merged with platform-owned values. It is deliberately
// unexported (not a persisted or public format; plan section 7) and holds no
// tags, ManagedBy, or force_destroy, which the Terraform module owns.
type resolved struct {
	AccountID                       string
	Region                          string
	Module                          string
	LogicalName                     string
	Environment                     string
	Owner                           string
	NoncurrentVersionExpirationDays int
	BucketName                      string // derived once; see composeBucketName
}

// resolve validates spec against standards and, only if both are valid,
// merges them. Validate is the sole gatekeeper: on any diagnostic the zero
// resolved value is returned, so callers cannot obtain a resolved
// configuration from invalid input.
func resolve(spec Spec, standards PlatformStandards) (resolved, []Diagnostic) {
	if diagnostics := Validate(spec, standards); len(diagnostics) != 0 {
		return resolved{}, diagnostics
	}
	return resolved{
		AccountID:                       standards.Spec.AWS.AccountID,
		Region:                          standards.Spec.AWS.Region,
		Module:                          standards.Spec.Bucket.Module,
		LogicalName:                     spec.Metadata.Name,
		Environment:                     spec.Spec.Environment,
		Owner:                           spec.Spec.Owner,
		NoncurrentVersionExpirationDays: standards.Spec.Bucket.NoncurrentVersionExpirationDays,
		BucketName: composeBucketName(
			standards.Spec.Bucket.BucketNamePrefix,
			spec.Metadata.Name,
			spec.Spec.Environment,
			standards.Spec.AWS.AccountID,
			standards.Spec.AWS.Region,
		),
	}, nil
}
