module github.com/steady-bytes/draft/services/tooling/grpc-call

go 1.25.3

replace github.com/steady-bytes/draft/api => ../../../api

// Local chassis is used (not a published version) because this service depends on
// chassis.Runtime.Effect (see pkg/chassis/effect.go) for the Garage catalog
// publish/retract pair — Effect hasn't been tagged in a published pkg/chassis
// release yet (confirmed against the latest tag, pkg/chassis/v0.6.1, which does not
// contain effect.go). Mirrors services/tooling/slack-notify/go.mod, which depends
// on it for the same reason.
replace github.com/steady-bytes/draft/pkg/chassis => ../../../pkg/chassis

// Local chassis's Logger interface (WithCallDepth) has to be matched by the local
// loggers implementation too — mirrors services/tooling/slack-notify/go.mod.
replace github.com/steady-bytes/draft/pkg/loggers => ../../../pkg/loggers

require (
	connectrpc.com/connect v1.16.2
	github.com/jhump/protoreflect v1.18.0
	github.com/steady-bytes/draft/api v1.0.0
	github.com/steady-bytes/draft/pkg/chassis v0.6.2
	github.com/steady-bytes/draft/pkg/loggers v0.2.6
	google.golang.org/grpc v1.66.2
	google.golang.org/protobuf v1.36.11
)

require (
	connectrpc.com/grpcreflect v1.2.0 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/cloudevents/sdk-go/binding/format/protobuf/v2 v2.15.0 // indirect
	github.com/fatih/color v1.14.1 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/hashicorp/go-hclog v1.5.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-msgpack/v2 v2.1.1 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hashicorp/raft v1.6.0 // indirect
	github.com/hashicorp/raft-boltdb/v2 v2.3.0 // indirect
	github.com/jhump/protoreflect/v2 v2.0.0-beta.1 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pelletier/go-toml/v2 v2.1.0 // indirect
	github.com/rs/cors v1.10.1 // indirect
	github.com/rs/zerolog v1.32.0 // indirect
	github.com/sagikazarmark/locafero v0.4.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/spf13/viper v1.18.2 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	go.etcd.io/bbolt v1.3.7 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20230905200255-921286631fa9 // indirect
	golang.org/x/net v0.26.0 // indirect
	golang.org/x/sys v0.21.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240604185151-ef581f913117 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
