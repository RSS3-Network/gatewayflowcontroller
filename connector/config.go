package connector

type Config struct {
	Listen string `yaml:"listen"`

	// Access Log Report
	KafkaBrokers []string `yaml:"kafka_brokers"`
	KafkaTopic   string   `yaml:"kafka_topic"`

	// State management
	EtcdEndpoints []string `yaml:"etcd_endpoints"`
	EtcdUsername  *string  `yaml:"etcd_username"`
	EtcdPassword  *string  `yaml:"etcd_password"`
}
