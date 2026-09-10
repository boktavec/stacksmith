variable "bucket_name" {
  description = "Final, fully composed S3 bucket name (already includes prefix, environment, account, and region). This module does not compose or reserve the name."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$", var.bucket_name))
    error_message = "bucket_name must be 3-63 characters, lowercase letters/digits/hyphens/dots, and start/end with a letter or digit."
  }
}

variable "name" {
  description = "Logical name of the bucket, used only for the Name tag (not the AWS bucket name)."
  type        = string

  validation {
    condition     = length(trimspace(var.name)) > 0 && length(var.name) <= 256 && can(regex("^[^\\x00-\\x1f\\x7f]*$", var.name))
    error_message = "name must be nonblank, at most 256 characters, and contain no control characters."
  }
}

variable "environment" {
  description = "Deployment environment (e.g. dev, prod), used for the Environment tag. Allowed values are enforced by platform standards outside this module."
  type        = string

  validation {
    condition     = length(trimspace(var.environment)) > 0
    error_message = "environment must be nonblank."
  }
}

variable "owner" {
  description = "Owning team, used for the Owner tag. Describes responsibility, not an identity or authorization grant."
  type        = string

  validation {
    condition     = length(trimspace(var.owner)) > 0 && length(var.owner) <= 256 && can(regex("^[^\\x00-\\x1f\\x7f]*$", var.owner))
    error_message = "owner must be nonblank, at most 256 characters, and contain no control characters."
  }
}

variable "noncurrent_version_expiration_days" {
  description = "Days after which a noncurrent object version expires. Platform-controlled; no developer override."
  type        = number

  validation {
    condition     = var.noncurrent_version_expiration_days > 0 && var.noncurrent_version_expiration_days == floor(var.noncurrent_version_expiration_days)
    error_message = "noncurrent_version_expiration_days must be a positive whole number."
  }
}
