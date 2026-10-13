package core

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxTagValueCharacters = 256

// Validate checks spec against standards using the semantic rules from
// docs/stacksmith-core-plan.md section 4. It assumes both inputs already
// parsed cleanly (see ParseSpec, ParseStandards). An empty result means spec
// is valid against standards.
func Validate(spec Spec, standards PlatformStandards) []Diagnostic {
	var diagnostics []Diagnostic
	add := func(document, field, message string) {
		diagnostics = append(diagnostics, Diagnostic{Document: document, Field: field, Message: message})
	}

	nameOK := validSlug(spec.Metadata.Name)
	if !nameOK {
		add("bucket spec", "metadata.name", "must be a lowercase ASCII slug; use letters and digits separated by single hyphens.")
	}
	environmentOK := validSlug(spec.Spec.Environment)
	if !environmentOK {
		add("bucket spec", "spec.environment", "must be a lowercase ASCII slug; use letters and digits separated by single hyphens.")
	}
	prefixOK := standards.Spec.Bucket.BucketNamePrefix == "" || validSlug(standards.Spec.Bucket.BucketNamePrefix)
	if !prefixOK {
		add("platform standards", "spec.bucket.bucketNamePrefix", "must be empty or a lowercase ASCII slug; use letters and digits separated by single hyphens.")
	}

	if len(standards.Spec.Environments) == 0 {
		add("platform standards", "spec.environments", "must contain at least one allowed environment.")
	}
	seenEnvironments := make(map[string]struct{}, len(standards.Spec.Environments))
	for i, environment := range standards.Spec.Environments {
		field := fmt.Sprintf("spec.environments[%d]", i)
		if !validSlug(environment) {
			add("platform standards", field, "must be a lowercase ASCII slug; use letters and digits separated by single hyphens.")
		}
		if _, exists := seenEnvironments[environment]; exists {
			add("platform standards", field, fmt.Sprintf("duplicates environment %q; keep each allowed environment once.", environment))
		} else {
			seenEnvironments[environment] = struct{}{}
		}
	}
	if environmentOK {
		if _, allowed := seenEnvironments[spec.Spec.Environment]; !allowed {
			add("bucket spec", "spec.environment", fmt.Sprintf("environment %q is not allowed; choose one listed in platform standards.", spec.Spec.Environment))
		}
	}

	accountOK := validAccountID(standards.Spec.AWS.AccountID)
	if !accountOK {
		add("platform standards", "spec.aws.accountId", "must be exactly 12 ASCII digits; quote the account ID to preserve leading zeros.")
	}
	regionOK := validRegion(standards.Spec.AWS.Region)
	if !regionOK {
		add("platform standards", "spec.aws.region", "must use AWS region-code syntax such as us-east-2; availability is not checked offline.")
	}
	if standards.Spec.Bucket.Module != "s3-private/v1" {
		add("platform standards", "spec.bucket.module", fmt.Sprintf("unsupported module %q; use %q.", standards.Spec.Bucket.Module, "s3-private/v1"))
	}
	if standards.Spec.Bucket.NoncurrentVersionExpirationDays <= 0 {
		add("platform standards", "spec.bucket.noncurrentVersionExpirationDays", "must be a positive whole number of days.")
	}

	owner := spec.Spec.Owner
	if strings.TrimSpace(owner) == "" {
		add("bucket spec", "spec.owner", "must not be blank; provide the owning team.")
	}
	if strings.TrimSpace(owner) != owner {
		add("bucket spec", "spec.owner", "must not have leading or trailing whitespace; remove surrounding spaces.")
	}
	for _, r := range owner {
		if unicode.IsControl(r) {
			add("bucket spec", "spec.owner", "must not contain control characters; provide a plain-text team name.")
			break
		}
	}
	if utf8.RuneCountInString(owner) > maxTagValueCharacters {
		add("bucket spec", "spec.owner", "exceeds the 256-character tag-value limit; shorten the owner value.")
	}

	if utf8.RuneCountInString(spec.Metadata.Name) > maxTagValueCharacters {
		add("bucket spec", "metadata.name", "exceeds the 256-character tag-value limit; shorten the logical name.")
	}
	if utf8.RuneCountInString(spec.Spec.Environment) > maxTagValueCharacters {
		add("bucket spec", "spec.environment", "exceeds the 256-character tag-value limit; shorten the environment value.")
	}

	if nameOK && environmentOK && prefixOK && accountOK && regionOK {
		bucketName := composeBucketName(standards.Spec.Bucket.BucketNamePrefix, spec.Metadata.Name, spec.Spec.Environment, standards.Spec.AWS.AccountID, standards.Spec.AWS.Region)
		if reason := invalidBucketName(bucketName); reason != "" {
			add("bucket spec", "metadata.name", fmt.Sprintf("composed bucket name %q %s; change the name or platform prefix.", bucketName, reason))
		}
	}

	return diagnostics
}

func validSlug(value string) bool {
	if value == "" {
		return false
	}
	previousHyphen := false
	for i, r := range value {
		if r == '-' {
			if i == 0 || i == len(value)-1 || previousHyphen {
				return false
			}
			previousHyphen = true
			continue
		}
		if !asciiLowerOrDigit(r) {
			return false
		}
		previousHyphen = false
	}
	return true
}

func asciiLowerOrDigit(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
}

func validAccountID(value string) bool {
	if len(value) != 12 {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func validRegion(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) < 3 || len(parts[0]) < 2 || !asciiLetters(parts[0]) || !asciiDigits(parts[len(parts)-1]) {
		return false
	}
	for _, part := range parts[1 : len(parts)-1] {
		if part == "" {
			return false
		}
		for _, r := range part {
			if !asciiLowerOrDigit(r) {
				return false
			}
		}
	}
	return true
}

func asciiLetters(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < 'a' || value[i] > 'z' {
			return false
		}
	}
	return true
}

func asciiDigits(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func composeBucketName(prefix, name, environment, accountID, region string) string {
	parts := []string{}
	if prefix != "" {
		parts = append(parts, prefix)
	}
	parts = append(parts, name, environment, accountID, region)
	return strings.Join(parts, "-")
}

func invalidBucketName(name string) string {
	if len(name) < 3 || len(name) > 63 {
		return "must be between 3 and 63 characters"
	}
	if name[0] == '-' || name[0] == '.' || name[len(name)-1] == '-' || name[len(name)-1] == '.' {
		return "must begin and end with a letter or digit"
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.') {
			return "contains a character not allowed in an S3 bucket name"
		}
	}
	if strings.Contains(name, "..") || strings.Contains(name, ".-") || strings.Contains(name, "-.") {
		return "contains an invalid adjacent period or hyphen"
	}
	for _, prefix := range []string{"xn--", "sthree-", "amzn-s3-demo-"} {
		if strings.HasPrefix(name, prefix) {
			return fmt.Sprintf("uses the reserved prefix %q", prefix)
		}
	}
	for _, suffix := range []string{"-s3alias", "--ol-s3", ".mrap", "--x-s3", "--table-s3"} {
		if strings.HasSuffix(name, suffix) {
			return fmt.Sprintf("uses the reserved suffix %q", suffix)
		}
	}
	if isIPAddress(name) {
		return "must not be formatted as an IPv4 address"
	}
	return ""
}

func isIPAddress(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		if !asciiDigits(part) || len(part) > 3 {
			return false
		}
		if len(part) > 1 && part[0] == '0' {
			return false
		}
		n, _ := strconv.Atoi(part)
		if n > 255 {
			return false
		}
	}
	return true
}
