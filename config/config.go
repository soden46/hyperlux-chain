package config

type Config struct {
	NodeID string
	Port   int
}

func LoadConfig() *Config {
	// TODO: load from file
	return &Config{NodeID: "node1", Port: 8080}
}
