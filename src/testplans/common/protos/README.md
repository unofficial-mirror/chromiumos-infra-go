# Generating protos

You can regenerate the protos in this directory following the instructions here:
https://developers.google.com/protocol-buffers/docs/reference/go-generated

Once you've installed protoc and the protoc-gen-go plugin, you can just run

```shell
protoc --proto_path=src/testplans/common/proto --go_out=src/testplans/common/proto src/testplans/common/proto/*.proto
```
