# Cycler

Cycler is a tool for the rapid iteration and modification of google storage
buckets. It allows the user to take advantage of object prefixes to parallelize
massively over the simple GS tools. Typically this prefix is '/'. It uses a
policy engine framework [Rego](https://www.openpolicyagent.org/docs/latest/policy-language/) on each [object's attributes](https://godoc.org/cloud.google.com/go/storage#ObjectAttrs) returned from list. It also has the ability to operate on various runtime and calculated values that can be passed to the policy engine. It also gathers statistics and produces a report in json or text.

Its configuration is specified [via protobuf](https://chromium.googlesource.com/chromiumos/infra/proto/+/HEAD/src/cycler/config.proto) (or the corresponding json) and has multiple implemented possible actions.

Logs are delivered which stat each object that is touched. These are uploaded to google storage or placed locally in compressed JSONL format. A simple audit of a storage bucket can be achieved by using the Noop effect and the `true.rego` policy.

## Actions

Currently Cycler contains the following actions:

* Move: Moves an object from one prefix & bucket to another.
* Duplicate: Duplicates an object from one prefix and bucket to another.
* Noop: Does nothing but gather statistics, useful if attempting to narrow down on a single object class.

Additionally planned actions include (potentially):

* ACL: Modifies the access control list properties of an object.
* Chill: Change the storage class of an object.
* Archive: Moves the object to an external service to GS (perhaps tape, etc).

## Invocation Options

```
./cycler --help

Cycler is a tool for rapid iteration of google storage buckets.

It is move effective in buckets that utilize a delimiter to indicate a hierarchical
topology. The most common example is unix like path names where the delimiter is '/'.

It provides an interface for generic effects to be mapped on to each
discovered object. For instance, to find the 'du' like tree of object
size, or to set acls, or even copy the object into another bucket.

  -alsologtostderr
    	log to standard error as well as files
  -bucket string
    	override the bucket name to operate on
  -iterJobs int
    	max number of object iterator jobs (default 2000)
  -jsonOutFile string
    	set if output should be written to a json file instead of plain text to stdout.
  -log_backtrace_at value
    	when logging hits line file:N, emit a stack trace
  -log_dir string
    	If non-empty, write log files in this directory
  -logtostderr
    	log to standard error instead of files
  -mutationAllowed
    	Must be set if the effect specified mutates objects.
  -prefixChannelDepth int
    	Size of the object prefix channel. (default 125000000)
  -prefixRoot string
    	the root prefix to iterate as path from root without decorations (e.g. asubdir/anotherone), defaults to root of bucket (the empty string)
  -retryCount int
    	Number of retries for an operation on any given object. (default 5)
  -runConfigPath string
    	the RunConfig input path (in binary or json representation).
  -stderrthreshold value
    	logs at or above this threshold go to stderr
  -v value
    	log level for V logs
  -vmodule value
    	comma-separated list of pattern=N settings for file-filtered logging
  -workUnitChannelDepth int
    	Size of the work unit channel. (default 4194304)
  -workerJobs int
    	number of object consumer jobs (default 2000)
```

## Example Invocation and Configuration

This invocation moves all the objects in a bucket that match a regex on their name as well as being of a certain age to another bucket. 

### Effect Config
```
{
  "run_log_configuration": {
      "destination_url": "gs://engeg-testing-chromeos-releases-2/logs",
      "chunk_size_bytes": 104857600,
      "channel_size": 10000,
      "persist_retries": 100,
      "max_unpersisted_logs": 10
  },

  "policy_effect_configuration": {
      "move": {
        "destination_bucket": "engeg-testing-chromeos-releases-2",
        "destination_prefix": "last_change/"
      },
      "policy_document_path": "examples/policies/unlikely_object_name.rego"
  },

  "stats_configuration": {
      "prefix_report_max_depth": 1,
      "age_days_histogram_options": {
          "num_buckets": 16,
          "growth_factor": 1.0,
          "base_bucket_size": 1.0,
          "min_value": 0
      },
      "size_bytes_histogram_options": {
          "num_buckets": 16,
          "growth_factor": 4.0,
          "base_bucket_size": 1.0,
          "min_value": 0
      }
  },

  "mutation_allowed" : true,

  "bucket": "engeg-testing-chromeos-releases"
}
```
### Policy Document: `not_firmware_policy.rego`
```
# The cycler executable will always load the package data.cycler
package cycler

skipped_prefix := false {
    re_match('gs://' + input.Bucket + '-firmware', input.attr.Name)
    re_match('(.*-test-ap|.*-test-ap-tryjob)')
}

# 'act' binding is the ultimate bool determination of if we should trigger
# the configuration supplied effect.
act := true {
    skipped_prefix
    input.ageDays > 180
}
```


### Command Line
`./cycler --runConfigPath ./examples/move_to_prefix.json --workerJobs 20000 --mutationAllowed -v 2`
