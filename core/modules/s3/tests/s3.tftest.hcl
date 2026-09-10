# Native Terraform module tests. mock_provider replaces the real AWS
# provider so these run entirely offline; no AWS resources are created.
mock_provider "aws" {}

variables {
  bucket_name                        = "acme-orders-assets-dev-123456789012-us-east-2"
  name                                = "orders-assets"
  environment                         = "dev"
  owner                               = "orders-team"
  noncurrent_version_expiration_days  = 30
}

run "tags_are_set_correctly" {
  command = plan

  assert {
    condition     = aws_s3_bucket.this.tags["Name"] == var.name
    error_message = "Name tag must equal the name input"
  }

  assert {
    condition     = aws_s3_bucket.this.tags["Environment"] == var.environment
    error_message = "Environment tag must equal the environment input"
  }

  assert {
    condition     = aws_s3_bucket.this.tags["Owner"] == var.owner
    error_message = "Owner tag must equal the owner input"
  }

  assert {
    condition     = aws_s3_bucket.this.tags["ManagedBy"] == "stacksmith"
    error_message = "ManagedBy tag must be the fixed literal 'stacksmith'"
  }
}

run "force_destroy_is_false" {
  command = plan

  assert {
    condition     = aws_s3_bucket.this.force_destroy == false
    error_message = "force_destroy must be false; there is no variable to override it"
  }
}

run "versioning_is_enabled" {
  command = plan

  assert {
    condition     = aws_s3_bucket_versioning.this.versioning_configuration[0].status == "Enabled"
    error_message = "versioning must be enabled with no disable switch"
  }
}

run "lifecycle_expires_noncurrent_versions" {
  command = plan

  assert {
    condition     = aws_s3_bucket_lifecycle_configuration.this.rule[0].status == "Enabled"
    error_message = "the noncurrent-version-expiration lifecycle rule must be enabled"
  }

  assert {
    condition     = aws_s3_bucket_lifecycle_configuration.this.rule[0].noncurrent_version_expiration[0].noncurrent_days == var.noncurrent_version_expiration_days
    error_message = "noncurrent version expiration days must equal the platform-provided input"
  }
}

run "public_access_is_fully_blocked" {
  command = plan

  assert {
    condition = alltrue([
      aws_s3_bucket_public_access_block.this.block_public_acls,
      aws_s3_bucket_public_access_block.this.block_public_policy,
      aws_s3_bucket_public_access_block.this.ignore_public_acls,
      aws_s3_bucket_public_access_block.this.restrict_public_buckets,
    ])
    error_message = "all four Block Public Access settings must be enabled"
  }
}

run "ownership_is_bucket_owner_enforced" {
  command = plan

  assert {
    condition     = aws_s3_bucket_ownership_controls.this.rule[0].object_ownership == "BucketOwnerEnforced"
    error_message = "object ownership must be BucketOwnerEnforced, disabling ACLs"
  }
}

run "encryption_is_sse_s3" {
  command = plan

  assert {
    condition     = one(one(aws_s3_bucket_server_side_encryption_configuration.this.rule).apply_server_side_encryption_by_default).sse_algorithm == "AES256"
    error_message = "default encryption must be SSE-S3 (AES256), not customer-managed KMS"
  }
}

run "policy_denies_insecure_transport" {
  command = plan

  assert {
    condition     = data.aws_iam_policy_document.deny_insecure_transport.statement[0].effect == "Deny"
    error_message = "the bucket policy statement must deny, not allow"
  }

  assert {
    condition     = contains(data.aws_iam_policy_document.deny_insecure_transport.statement[0].actions, "s3:*")
    error_message = "the deny statement must cover s3 actions on the bucket and its objects"
  }

  assert {
    condition     = one(data.aws_iam_policy_document.deny_insecure_transport.statement[0].condition).variable == "aws:SecureTransport"
    error_message = "the deny condition must key off aws:SecureTransport"
  }

  assert {
    condition     = contains(one(data.aws_iam_policy_document.deny_insecure_transport.statement[0].condition).values, "false")
    error_message = "the condition must deny when SecureTransport is false, i.e. non-TLS requests"
  }
}
