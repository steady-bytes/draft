package chassis

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

type (
	Config interface {
		Name() string
		Domain() string
		NodeID() string
		Title() string
		Env() string
		Reader
		Writer
		Entrypoint() string
	}
	Reader interface {
		Get(key string) interface{}
		GetString(key string) string
		GetBool(key string) bool
		GetInt(key string) int
		GetInt32(key string) int32
		GetInt64(key string) int64
		GetUint(key string) uint
		GetUint16(key string) uint16
		GetUint32(key string) uint32
		GetUint64(key string) uint64
		GetFloat64(key string) float64
		GetTime(key string) time.Time
		GetDuration(key string) time.Duration
		GetIntSlice(key string) []int
		GetStringSlice(key string) []string
		GetStringMap(key string) map[string]interface{}
		GetStringMapString(key string) map[string]string
		GetStringMapStringSlice(key string) map[string][]string
		GetSizeInBytes(key string) uint
		Unmarshal(rawVal interface{}, opts ...viper.DecoderConfigOption) error
		UnmarshalKey(key string, rawVal interface{}, opts ...viper.DecoderConfigOption) error
		SetDefault(key string, value any)
	}
	Writer interface {
		Set(key string, value any)
		SetDefault(key string, value any)
		SetAndWrite(key string, value any) error
		WriteConfig() error
	}

	config struct {
		*viper.Viper
		mu sync.Mutex
	}
)

var configSingleton *config

// TODO -> Read config from the key/value store and not from a local static file.
func LoadConfig() Config {
	v := viper.New()
	setDefaults(v)
	v.SetEnvPrefix("DRAFT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	configPath := v.GetString("config")
	if configPath == "" {
		configPath = "./config.yaml"
		fmt.Printf("using default config path: %s\n", configPath)
	}
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		// yes, we actually want to panic here as without a config there's nothing we can do
		// TODO: make this optional?
		panic(fmt.Errorf("failed to read in config: %s", err.Error()))
	}
	configSingleton = &config{v, sync.Mutex{}}
	return configSingleton
}

func setDefaults() {
	viper.SetDefault("service.network.port", 8090)
	viper.SetDefault("service.network.bind_address", "0.0.0.0")
	viper.SetDefault("service.network.advertise_address", "127.0.0.1")
	viper.SetDefault("service.env", "local")
	viper.SetDefault("service.logging.level", "info")
}

func (c *config) Name() string {
	return c.GetString("service.name")
}

func (c *config) Domain() string {
	return c.GetString("service.domain")
}

func (c *config) NodeID() string {
	return c.GetString("raft.node-id")
}

func (c *config) Title() string {
	return fmt.Sprintf("%s_%s", c.Name(), c.NodeID())
}

func (c *config) Entrypoint() string {
	return c.GetString("service.entrypoint")
}

func (c *config) Env() string {
	return c.GetString("service.env")
}

func GetConfig() Config {
	if configSingleton == nil {
		LoadConfig()
	}
	return configSingleton
}

func (c *config) SetAndWrite(key string, value any) error {
	c.Set(key, value)
	return c.WriteConfig()
}

func (c *config) WriteConfig() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Viper.WriteConfig()
}
