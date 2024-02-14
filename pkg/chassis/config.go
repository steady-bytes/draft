package chassis

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type (
	Config interface {
		Name() string
		NodeID() string
		Title() string
		Env() string
		Reader
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
	}

	config struct {
		*viper.Viper
	}
)

// TODO -> Read config from the key/value store and not from a local static file.
func LoadConfig() Config {
	viper.AutomaticEnv()
	viper.SetConfigFile("./config.yaml")
	if err := viper.ReadInConfig(); err != nil {
		// yes, we actually want to panic here as without a config there's nothing we can do
		panic(fmt.Errorf("failed to read in config: %s", err.Error()))
	}
	return &config{viper.GetViper()}
}

func (c *config) Name() string {
	return c.GetString("service.name")
}

func (c *config) NodeID() string {
	return c.GetString("service.node_id")
}

func (c *config) Title() string {
	return fmt.Sprintf("%s_%s", c.Name(), c.NodeID())
}

func (c *config) Env() string {
	return c.GetString("service.env")
}
