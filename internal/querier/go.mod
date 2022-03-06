module querier

go 1.17

replace (
	api v0.0.1 => ../../api/gen/go
	commet v0.0.1 => ../../pkg/commet
)

require (
	api v0.0.1
	commet v0.0.1
	github.com/jinzhu/gorm v1.9.16
	google.golang.org/grpc v1.44.0
)

require (
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	golang.org/x/net v0.0.0-20220114011407-0dd24b26b47d // indirect
	golang.org/x/sys v0.0.0-20220114195835-da31bd327af9 // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/genproto v0.0.0-20220204002441-d6cc3cc0770e // indirect
	google.golang.org/protobuf v1.27.1 // indirect
)
