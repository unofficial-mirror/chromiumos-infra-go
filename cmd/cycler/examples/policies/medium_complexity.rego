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

# A skipped_prefix for the image archive is one that is a firmware
# or an access point test or access point test tryjob. This configuration
# was copied without much introspection from the older purgejob. We should
# revisit this eventually to ensure we still require the exceptions.
skipped_prefix := false {
    # TODO(engeg@): we have trybot-.*-firmware, which should be deleted but
    # isn't under these rules.
    re_match('gs://' + input.Bucket + '-firmware', input.attr.Name)
    re_match('(.*-test-ap|.*-test-ap-tryjob)')
}

# 'act' binding is the ultimate bool determination of if we should trigger
# the configuration supplied effect.
act := true {
    skipped_prefix
    input.ageDays > 180
}
