package main

import (
	"fmt"
	"os"
	"time"

	"go.bug.st/serial"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Strips []StripConfig `yaml:"strips"`
}

type StripConfig struct {
	Zones []ZoneConfig `yaml:"zones"`
}

type ZoneConfig struct {
	Name string `yaml:"name"`
	On   uint16 `yaml:"on"`
	Off  uint16 `yaml:"off"`
}

type ZoneIndex struct {
	Strip uint8
	Zone  uint8
}

func LoadConfig() (*Config, error) {
	bytes, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("error reading config.yaml: %w", err)
	}

	var config Config
	err = yaml.Unmarshal(bytes, &config)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling config.yaml: %w", err)
	}

	return &config, nil
}

type LEDController struct {
	port      serial.Port
	zoneCache map[string]ZoneIndex
}

func NewLEDController(serialPath string) (*LEDController, error) {
	serialMode := &serial.Mode{
		BaudRate: 115200,
	}
	serialPort, err := serial.Open(serialPath, serialMode)
	if err != nil {
		return nil, fmt.Errorf("error opening serial port: %w", err)
	}

	go func() {
		buf := make([]byte, 4096)
		serialPort.SetReadTimeout(10 * time.Millisecond)

		for {
			bytes, err := serialPort.Read(buf)
			if err != nil {
				fmt.Println(err)
				break
			}

			fmt.Print(string(buf[:bytes]))
		}
	}()

	return &LEDController{
		port:      serialPort,
		zoneCache: map[string]ZoneIndex{},
	}, nil
}

func (c *LEDController) Configure(config *Config) error {
	c.zoneCache = map[string]ZoneIndex{}

	_, err := c.port.Write([]byte{'R'})
	if err != nil {
		return err
	}

	for stripNum, strip := range config.Strips {
		for zoneNum, zone := range strip.Zones {
			if zone.Name == "" {
				continue
			}

			c.zoneCache[zone.Name] = ZoneIndex{
				Strip: uint8(stripNum),
				Zone:  uint8(zoneNum),
			}

			if zone.Off > 0 {
				_, err = c.port.Write([]byte{
					'D',
					uint8(stripNum),
					uint8(zoneNum),
					uint8(zone.Off >> 8),
					uint8(zone.Off & 0xFF),
				})
				if err != nil {
					return err
				}
			}

			_, err = c.port.Write([]byte{
				'L',
				uint8(stripNum),
				uint8(zoneNum),
				uint8(zone.On >> 8),
				uint8(zone.On & 0xFF),
			})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *LEDController) SetZoneEffectRaw(strip, zone uint8, effect uint32) error {
	_, err := c.port.Write([]byte{
		'E',
		strip,
		zone,
		uint8(effect >> 24),
		uint8((effect >> 16) & 0xFF),
		uint8((effect >> 8) & 0xFF),
		uint8(effect & 0xFF),
	})

	return err
}

func (c *LEDController) SetZoneEffect(zoneName string, effect uint32) error {
	zone, ok := c.zoneCache[zoneName]

	if ok {
		err := c.SetZoneEffectRaw(zone.Strip, zone.Zone, effect)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *LEDController) Close() error {
	return c.port.Close()
}
