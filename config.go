package dwgd

// A Config represents the configuration of an instance of a dwgd driver.
type Config struct {
	Db       string // path to the database
	Verbose  bool   // whether to print debug logs or not
	Rootless bool   // whether to run in rootless compatibility mode or not
}

func NewConfig() *Config {
	return &Config{
		Db:       "/var/lib/dwgd.db",
		Verbose:  false,
		Rootless: true,
	}
}
