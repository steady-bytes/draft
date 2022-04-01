package commet

import (
	"fmt"

	"github.com/spf13/viper"
)

const (
	// PANIC: failure to read configuration
	ErrorCode int16 = 1
)

// config
type Config struct {
	Service  *Service
	Repos    map[string]Repo
	Gateways map[string]Gateway
}

// Service
type Service struct {
	// Name is also considered the services address
	// default "localhost"
	Name string
	// Port that is accepting requests on
	RPCPort  int32
	HTTPPort int32
}

// gateway
type Gateway struct {
	// A service can have n number of GrpcClient connections
	GRPC GrpcClient
}

type GrpcClient struct {
	Type    string
	Address string
	Port    int
}

// repo
type Repo struct {
	// postgres is the only client that is currently configured
	Postgres PostgresConnectionConfig

	// @TODO -> Implement ScyllaDB
}

// PostgresConnectionConfig
type PostgresConnectionConfig struct {
	Type     string
	Protocol string
	User     string
	Domain   string
	Port     int
	Server   string
	LogMode  bool
	SSL      bool
	Migrate  bool
}

func NewConfig(name string, rpcPort, httpPort int32) *Config {
	config := &Config{
		Service: &Service{
			Name:     name,
			RPCPort:  rpcPort,
			HTTPPort: httpPort,
		},
		Repos:    readRepoConfig(),
		Gateways: readGatewayConfig(),
	}

	return config
}

// readGatewayConfig: A utility method that is use to retrieve a gateway from the configuration file
// @NOTE: This does require updates if the Gateway struct changes
func readGatewayConfig() map[string]Gateway {
	// read in gateways from config file using viper
	configGateways := viper.GetStringMap("Gateways")

	// create a map to store Gateway structs in config
	gateways := make(map[string]Gateway, len(configGateways))

	for k, v := range configGateways {
		var newGateway Gateway
		// match the gateway type
		if k == USERS || k == AUTHORIZATION {
			for key, val := range v.(map[string]interface{}) {
				switch key {
				case "gatewaytype":
					newGateway.GRPC.Type = val.(string)
				case "port":
					newGateway.GRPC.Port = val.(int)
				case "address":
					newGateway.GRPC.Address = val.(string)
				default:
					fmt.Sprintf("key: [%s] is not found in %s gateway options", key, USERS)
					// panic(ErrorCode)
				}
			}
		} else {
			fmt.Println("gateway type not found")
			panic(ErrorCode)
		}

		gateways[k] = newGateway
	}

	return gateways
}

// gateway configuration options
const (
	USERS         = "users"
	AUTHORIZATION = "authorization"
)

func readRepoConfig() map[string]Repo {
	configRepos := viper.GetStringMap("Repos")

	repos := make(map[string]Repo, len(configRepos))

	for k, v := range configRepos {
		var newRepo Repo

		if k == Postgres.String() {
			for key, val := range v.(map[string]interface{}) {
				switch key {
				case "dbtype":
					newRepo.Postgres.Type = val.(string)
				case "protocol":
					newRepo.Postgres.Protocol = val.(string)
				case "user":
					newRepo.Postgres.User = val.(string)
				case "domain":
					newRepo.Postgres.Domain = val.(string)
				case "port":
					newRepo.Postgres.Port = val.(int)
				case "server":
					newRepo.Postgres.Server = val.(string)
				case "logmode":
					newRepo.Postgres.LogMode = val.(bool)
				case "ssl":
					newRepo.Postgres.SSL = val.(bool)
				case "migrate":
					newRepo.Postgres.Migrate = val.(bool)
				default:
					fmt.Sprintf("key: [%s] is not found in %s repo options", key, Postgres.String())
				}
			}
		} else if k == Scylla.String() {
			fmt.Println("IMPLEMENT SCYLLA")
			panic(ErrorCode)
		} else {
			fmt.Println("repo type not found")
			panic(ErrorCode)
		}

		repos[k] = newRepo
	}

	return repos
}
