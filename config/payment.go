package config

import "time"

type Payment struct {
	EasyPay EasyPay `mapstructure:"epay"`
}

type EasyPay struct {
	Enable    bool          `mapstructure:"enable"`
	BaseURL   string        `mapstructure:"base-url"`
	Pid       string        `mapstructure:"pid"`
	Key       string        `mapstructure:"key"`
	NotifyURL string        `mapstructure:"notify-url"`
	ReturnURL string        `mapstructure:"return-url"`
	Timeout   time.Duration `mapstructure:"timeout"`
}
