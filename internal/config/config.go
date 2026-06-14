package config

import (
	"strings"

	"github.com/spf13/viper"
)

// Go version of the YAML file; a struct
// Helps viper load the YAML file
type Config struct {
	App struct {
		Name string `mapstructure:"name"`
		Env  string `mapstructure:"env"`
	} `mapstructure:"app"`

	Server struct {
		Host                string `mapstructure:"host"`
		Port                int    `mapstructure:"port"`
		ReadTimeoutSeconds  int    `mapstructure:"read_timeout_seconds"`
		WriteTimeoutSeconds int    `mapstructure:"write_timeout_seconds"`
	} `mapstructure:"server"`

	Redis struct {
		Addr     string `mapstructure:"addr"`
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
		DB       int    `mapstructure:"db"`
	} `mapstructure:"redis"`

	Cache struct {
		Enabled             bool    `mapstructure:"enabled"`
		SimilarityThreshold float64 `mapstructure:"similarity_threshold"`
		TTLSeconds          int     `mapstructure:"ttl_seconds"`
	} `mapstructure:"cache"`

	Embedder struct {
		Provider string `mapstructure:"provider"`
		Model    string `mapstructure:"model"`
		BaseURL  string `mapstructure:"base_url"`
	} `mapstructure:"embedder"`

	Normalizer struct {
		FillerPhrases []string `mapstructure:"filler_phrases"`
	} `mapstructure:"normalizer"`

	LLM struct {
		Provider string `mapstructure:"provider"`
		BaseURL  string `mapstructure:"base_url"`
		APIKey   string `mapstructure:"api_key"`
		Model    string `mapstructure:"model"`
	} `mapstructure:"llm"`

	Logging struct {
		Level  string `mapstructure:"level"`
		Format string `mapstructure:"format"`
	} `mapstructure:"logging"`

	Metrics struct {
		Enabled bool   `mapstructure:"enabled"`
		Path    string `mapstructure:"path"`
	} `mapstructure:"metrics"`

	Health struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"health"`

	Policy struct {
		MinResponseChar int      `mapstructure:"min_response_chat"`
		MinTotalTokens  int      `mapstructure:"min_total_tokens"`
		HedgingPhrases  []string `mapstructure:"hedging_phrases"`
	} `mapstructure:"policy"`
}

// Reads YAML converts it to config struct
func LoadConfig() (*Config, error) {
	v := viper.New() //New instance of viper

	v.SetConfigName("config.local") //Look for a file called config.local
	v.SetConfigType("yaml")         //yaml file
	v.AddConfigPath("./configs")    //In configs dir

	//If reading the YAML file fails; set these as default
	/*
		v.SetDefault("app.name", "llm-cache")
		v.SetDefault("app.env", "local")

		v.SetDefault("server.host", "0.0.0.0")
		v.SetDefault("server.port", 8080)
		v.SetDefault("server.read_timeout_seconds", 30)
		v.SetDefault("server.write_timeout_seconds", 30)

		v.SetDefault("health.path", "/healthz")

		v.SetDefault("metrics.enabled", true)
		v.SetDefault("metrics.path", "/metrics")

		v.SetDefault("redis.addr", "localhost:6379")
		v.SetDefault("redis.username", "")
		v.SetDefault("redis.password", "")
		v.SetDefault("redis.db", 0)

		v.SetDefault("cache.enabled", true)
		v.SetDefault("cache.similarity_threshold", 0.85)
		v.SetDefault("cache.ttl_seconds", 86400)

		v.SetDefault("logging.level", "debug")
		v.SetDefault("logging.format", "console")
	*/

	//Lets me overide YAML values with env.local; useful for customization
	//LL_API_KEY <-> llm.api_key; SERVER_PORT <-> server.port
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	//loading the config file
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	//Convert the YAML to the config struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
