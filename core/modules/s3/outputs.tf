output "bucket_name" {
  description = "The bucket's actual AWS name."
  value       = aws_s3_bucket.this.id
}

output "bucket_arn" {
  description = "The bucket's ARN."
  value       = aws_s3_bucket.this.arn
}
