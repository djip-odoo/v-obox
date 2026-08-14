package config

const AppName = "EposProxy"

// OdooConfig holds the credentials and identifiers for connecting to an Odoo instance.
type OdooConfig struct {
	DbURL        string `json:"db_url,omitempty"`
	Token        string `json:"token,omitempty"`
	DbUUID       string `json:"db_uuid,omitempty"`
}

// AppConfig represents the root application configuration stored on disk.
type AppConfig struct {
	AppID       string     `json:"app_id"`
	Port        int        `json:"port"`
	LANPrinters []string   `json:"lan_printers,omitempty"`
	Odoo        OdooConfig `json:"odoo,omitempty"`
}

func defaults() AppConfig {
	return AppConfig{
		Port:        0,
		LANPrinters: []string{},
	}
}
