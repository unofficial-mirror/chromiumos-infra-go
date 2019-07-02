# Copyright 2019 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

def CheckGolangRunTests(input_api, output_api):
  try:
    test_output = input_api.subprocess.check_output(['./run_tests.sh'])
  except Exception as error:
    return [
        output_api.PresubmitError(
            'run_tests.sh failed.\noutput:%s\n' % error.output)
    ]
  return []


def CommonChecks(input_api, output_api):
  results = []

  # ./run_tests.sh
  results.extend(CheckGolangRunTests(input_api, output_api))

  return results


#TODO verbose flag
CheckChangeOnUpload = CommonChecks
CheckChangeOnCommit = CommonChecks
