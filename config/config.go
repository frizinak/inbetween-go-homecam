package config

import (
	"encoding/json"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

var (
	touchMap = map[rune]byte{
		'↑': 8,
		'↓': 4,
		'→': 2,
		'←': 1,
		'↖': 8 | 1,
		'↗': 8 | 2,
		'↙': 4 | 1,
		'↘': 4 | 2,
	}
)

type TouchPassword []byte

func (t TouchPassword) String() string {
	rev := make(map[byte]rune, len(touchMap))
	for i := range touchMap {
		rev[touchMap[i]] = i
	}

	str := make([]rune, len(t))
	for i := range t {
		str[i] = rev[t[i]]
	}

	return string(str)
}

type Config struct {
	Address          string
	Device           string
	Password         string
	TouchPassword    interface{}
	rawTouchPassword TouchPassword
	MaxPeers         int
	Quality          Quality
}

func (c Config) RawTouchPassword() TouchPassword {
	if c.rawTouchPassword != nil {
		return c.rawTouchPassword
	}

	switch v := c.TouchPassword.(type) {
	case []byte:
		c.rawTouchPassword = v
		return v
	case string:
		bts := make([]byte, 0, len(v)*2)
		for _, c := range v {
			b, ok := touchMap[c]
			if !ok {
				return nil
			}
			bts = append(bts, b)
		}
		return bts
	}

	return nil
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

func (q Quality) MinimumFPS() int                  { return q.MinFPS }
func (q Quality) MaximumFPS() int                  { return q.MaxFPS }
func (q Quality) MinimumJPEGQuality() int          { return q.MinJPEGQuality }
func (q Quality) MaximumJPEGQuality() int          { return q.MaxJPEGQuality }
func (q Quality) DesiredTotalThroughput() float64  { return q.MaxKilobytesPerSecond * 1024 }
func (q Quality) DesiredClientThroughput() float64 { return q.MaxKilobytesPerSecondPerClient * 1024 }
func (q Quality) MinimumResolution() int           { return q.MinWidth * q.MinHeight }
func (q Quality) MaximumResolution() int           { return q.MaxWidth * q.MaxHeight }

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
	if err = d.Decode(c); err != nil {
		return *c, err
	}

	if c.RawTouchPassword() == nil {
		return *c, errors.New("Invalid touchpassword type")
	}

	return *c, nil
}

func EnsureConfig(file string) error {
	var randPass string
	chars := "abcdefghijklmnopqrstuvxyzABCDEFGHIJKLMNOPQRSTUVXYZ0123456789-!@#$%^&*-=(){}"

	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 60; i++ {
		randPass += string(chars[rand.Intn(len(chars))])
	}

	c := Config{
		Address:       "127.0.0.1:1234",
		Password:      randPass,
		TouchPassword: []byte{8, 8, 8, 8, 8},
		Device:        "/dev/video0",
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
