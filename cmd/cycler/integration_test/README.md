# General Information

These tests will use your default GCP credentials and will create buckets and
objects during the course of the run. These will be cleaned up with best effort
however there is some possibility that artifacts will remain. These are real
operations as well, so any applicable charges will be incured.

# Running tests

To run a single test execute one of the `test_testname.sh` files on the command
line. To run all tests execute the `run_all.sh` script.

The tests are all ran with the top level directory (i.e. `./integration_tests`)
as the cwd. All effects and files should be relative to this.

# Contact

Contact engeg@ or the chromeos-continuous-integration-team@google.com for support
or more information about cycler, these tests, or any other queries.
