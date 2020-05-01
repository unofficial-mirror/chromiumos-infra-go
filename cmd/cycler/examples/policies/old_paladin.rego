# The cycler executable will always load the package data.cycler
package cycler

# The global input should be a google storage ObjectAttrs struct.
# ObjectAttrs represents the metadata for a Google Cloud Storage (GCS) object.
# type ObjectAttrs struct {
#   Bucket string
#   Name string
#   ContentType string
#   ContentLanguage string
#   CacheControl string
#   EventBasedHold bool
#   TemporaryHold bool
#   RetentionExpirationTime time.Time
#   ACL []ACLRule
#   PredefinedACL string
#   Owner string
#   Size int64
#   ContentEncoding string
#   ContentDisposition string
#   MD5 []byte
#   CRC32C uint32
#   MediaLink string
#   Metadata map[string]string
#   Generation int64
#   Metageneration int64
#   StorageClass string
#   Created time.Time
#   Deleted time.Time
#   Updated time.Time
#   CustomerKeySHA256 string
#   KMSKeyName string
#   Prefix string
#   Etag string
# }

# 'act' binding is the ultimate bool determination of if we should trigger
# the configuration supplied effect.
act := true {
    re_match(".+?-paladin(-tryjob)?/.*$", input.attr.Name)
    input.ageDays > 180
}
