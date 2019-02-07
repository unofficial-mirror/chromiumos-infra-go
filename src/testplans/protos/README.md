# Generating protos

You can regenerate the protos in this directory following the instructions here:
https://developers.google.com/protocol-buffers/docs/reference/go-generated

Once you've installed protoc and the protoc-gen-go plugin, you can just run

```shell
protoc --proto_path=src/testplans/common/protos --go_out=src/testplans/common/protos src/testplans/common/protos/*.proto
```
