package dwgd

type Config struct {
	Db       string
	Verbose  bool
	Rootless bool
}

func NewConfig() *Config {
	return &Config{
		Db:       "/var/lib/dwgd.db",
		Verbose:  false,
		Rootless: true,
	}
}
