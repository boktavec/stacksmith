output "bucket_name" {
  description = "The bucket's actual AWS name."
  value       = module.bucket.bucket_name
}

output "bucket_arn" {
  description = "The bucket's ARN."
  value       = module.bucket.bucket_arn
}
