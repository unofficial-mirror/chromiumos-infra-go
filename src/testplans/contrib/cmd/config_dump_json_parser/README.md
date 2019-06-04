# Config dump JSON parser

This is a dev-only tool for parsing test-related data out of a fully-expanded Chrome OS
`config_dump.json`.

## Usage

This assumes you have a Chromium OS repo checkout at ~/chromiumos.

Firstly, create a fully expanded config_dump.json:

```shell
mkdir ${HOME}/tmp
${HOME}/chromiumos/chromite/bin/cbuildbot_view_config --full > ${HOME}/tmp/config_dump.json
```

Now run the script:

```shell
go run contrib/cmd/config_dump_json_parser/main.go --config_dump_json_path=${HOME}/tmp/config_dump.json
```
