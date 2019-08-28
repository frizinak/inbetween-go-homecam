package config

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/frizinak/inbetween-go-homecam/server"
)

type Config struct {
	Address  string
	Device   string
	Password string
	Quality  Quality
}

type Quality struct {
	MinFPS int
	MaxFPS int

	MinJPEGQuality int
	MaxJPEGQuality int

	MaxKilobytesPerSecond          float64
	MaxKilobytesPerSecondPerClient float64

	MinWidth  int
	MinHeight int
	MaxWidth  int
	MaxHeight int
}

func (q Quality) ToServerConfig() server.QualityConfig {
	return server.QualityConfig{
		MinFPS: q.MinFPS,
		MaxFPS: q.MaxFPS,

		MinJPEGQuality: q.MinJPEGQuality,
		MaxJPEGQuality: q.MaxJPEGQuality,

		DesiredTotalThroughput:  q.MaxKilobytesPerSecond * 1024,
		DesiredClientThroughput: q.MaxKilobytesPerSecondPerClient * 1024,

		MinResolution: q.MinWidth * q.MinHeight,
		MaxResolution: q.MaxWidth * q.MaxHeight,
	}
}

func DefaultConfigFile() (string, error) {
	home, err := os.UserHomeDir()
	return filepath.Join(home, ".config", "homecam", "config.json"), err
}

func LoadConfig(file string) (Config, error) {
	c := &Config{}
	f, err := os.Open(file)
	if err != nil {
		return *c, err
	}
	d := json.NewDecoder(f)
	return *c, d.Decode(c)
}

func EnsureConfig(file string) error {
	var randPass string
	chars := "abcdefghijklmnopqrstuvxyzABCDEFGHIJKLMNOPQRSTUVXYZ0123456789-!@#$%^&*-=(){}"

	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 60; i++ {
		randPass += string(chars[rand.Intn(len(chars))])
	}

	c := Config{
		Address:  "127.0.0.1:1234",
		Password: randPass,
		Device:   "/dev/video0",
		Quality: Quality{
			MinFPS: 5,
			MaxFPS: 20,

			MinJPEGQuality: 30,
			MaxJPEGQuality: 100,

			MinWidth:  480,
			MinHeight: 320,
			MaxWidth:  1024,
			MaxHeight: 768,

			MaxKilobytesPerSecond:          1200,
			MaxKilobytesPerSecondPerClient: 200,
		},
	}

	dirs := filepath.Dir(file)
	os.MkdirAll(dirs, 0755)
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "    ")
	return enc.Encode(c)
}
