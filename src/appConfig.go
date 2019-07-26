package main

import "github.com/BurntSushi/toml"

/*Config 配置对象*/
type Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	Port            int
	JWTSecret       string
	FileSizeLimit   int
	CacheSize       int
	UseHTTP2        bool
	CertPath        string
	KeyPath         string
	FileTypes       string
}

/*AppConfig 系统配置*/
var AppConfig Config

func init() {
	if _, err := toml.DecodeFile("config.toml", &AppConfig); err != nil {
		panic(err)
	}
}
