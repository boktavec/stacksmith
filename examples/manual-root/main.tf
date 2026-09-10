# Literal example values. This root exists to prove the module composes
# correctly by hand, before anything generates it. The account ID below is a
# placeholder, not a real account; replace it before ever running this
# against actual AWS.
module "bucket" {
  source = "../../core/modules/s3"

  bucket_name                        = "acme-orders-assets-dev-123456789012-us-east-2"
  name                               = "orders-assets"
  environment                        = "dev"
  owner                              = "orders-team"
  noncurrent_version_expiration_days = 30
}
