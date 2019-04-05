# Test plan generator sample

So you want to try running the test plan generator locally? Cool! Alright,
you're going to have to do a bit of setup, and you'll have to be a Googler.

1. Have the test_planner repo (you're in it now) checked out (see
   https://chromium.googlesource.com/chromiumos/infra/test_planner).
1. Have a local chromiumos repo checkout.
1. Have depot_tools (in particular, the repo command) on your PATH.
1. Have a local chromeos/infra/config checkout (see
   https://chrome-internal.googlesource.com/chromeos/infra/config).
1. Have Golang >=1.12 installed.

OK, now edit gen_test_plan_input.json and replace the REPLACE strings.

This might look something like

```json
{
  "chromiumos_checkout_root": "/home/sean/chromiumos",
  "buildbucket_build_path": [
    {
      "file_path": "/home/sean/test_planner/src/testplans/cmd/test_plan_generator/sample/build_bucket_build_1.cfg"
    },
    {
      "file_path": "/home/sean/test_planner/src/testplans/cmd/test_plan_generator/sample/build_bucket_build_2.cfg"
    }
  ],
  "source_tree_config_path": "/home/sean/chromeos-infra-config/config/testingconfig/generated/source_tree_test_config.cfg",
  "target_test_requirements_path": "/home/sean/chromeos-infra-config/config/testingconfig/generated/target_test_requirements.cfg"
}
```

Alright, now you'll need to get OAuth credentials to run the program:

```shell
# Get to this folder in the repo
# cd test_planner/src/testplans/
go run cmd/test_plan_generator/main.go auth-login
```

And now you can actually run it:

```shell
go run cmd/test_plan_generator/main.go gen-test-plan \
    --input_json=$PWD/cmd/test_plan_generator/sample/input.json \
    --output_json=$PWD/cmd/test_plan_generator/sample/output.json
```
