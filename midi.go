package main

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/smf"
	"gopkg.in/yaml.v3"
)

type SongMapping struct {
	Keys     map[string]string `yaml:"keys"`
	Channels map[uint8]string  `yaml:"channels"`
}

func LoadMapping(filename string) (*SongMapping, error) {
	bytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error loading mapping file %s: %w", filename, err)
	}

	var mapping SongMapping
	err = yaml.Unmarshal(bytes, &mapping)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling mapping file %s: %w", filename, err)
	}

	return &mapping, nil
}

type noteStatus struct {
	channel uint8
	effect  uint32
}
type noteStacker [128][]noteStatus

func (n *noteStacker) Play(key, channel uint8, effect uint32) uint32 {
	found := false

	for i, status := range n[key] {
		if status.channel == channel {
			found = true
		}

		if found && i < len(n[key])-1 {
			n[key][i] = n[key][i+1]
		}
	}

	if found {
		n[key][len(n[key])-1] = noteStatus{
			channel: channel,
			effect:  effect,
		}
	} else {
		n[key] = append(n[key], noteStatus{
			channel: channel,
			effect:  effect,
		})
	}

	return effect
}

func (n *noteStacker) Release(key, channel uint8) uint32 {
	found := false

	for i, status := range n[key] {
		if status.channel == channel {
			found = true
		}

		if found && i < len(n[key])-1 {
			n[key][i] = n[key][i+1]
		}
	}

	if found {
		n[key] = n[key][:len(n[key])-1]
	}

	if len(n[key]) > 0 {
		return n[key][len(n[key])-1].effect
	} else {
		return 0
	}
}

type LightPlayer struct {
	controller *LEDController
	mapping    *SongMapping
	midiFile   *smf.SMF

	noteStatuses noteStacker
}

func LoadLightPlayer(midiFileName string, controller *LEDController) (*LightPlayer, error) {
	mappingFileName := strings.TrimSuffix(midiFileName, "mid") + "map.yaml"
	mapping, err := LoadMapping(mappingFileName)
	if err != nil {
		return nil, err
	}

	midiFile, err := smf.ReadFile(midiFileName)
	if err != nil {
		return nil, err
	}

	return &LightPlayer{
		controller: controller,
		mapping:    mapping,
		midiFile:   midiFile,
	}, nil
}

func (l *LightPlayer) Play() {
	noteEvents := []smf.TrackEvent{}

	for trackNum, track := range l.midiFile.Tracks {
		var absTicks int64

		for _, event := range track {
			absTicks += int64(event.Delta)

			if !(event.Message.Type().Is(midi.NoteOnMsg) || event.Message.Type().Is(midi.NoteOffMsg)) {
				continue
			}

			trackEvent := smf.TrackEvent{
				Event:           event,
				TrackNo:         trackNum,
				AbsTicks:        absTicks,
				AbsMicroSeconds: l.midiFile.TimeAt(absTicks),
			}

			noteEvents = append(noteEvents, trackEvent)
		}
	}

	slices.SortFunc(noteEvents, func(a, b smf.TrackEvent) int {
		if a.AbsTicks > b.AbsTicks {
			return 1
		} else if a.AbsTicks < b.AbsTicks {
			return -1
		} else {
			return 0
		}
	})

	start := time.Now()

	for _, noteEvent := range noteEvents {
		elapsed := time.Since(start)
		diff := (time.Microsecond * time.Duration(noteEvent.AbsMicroSeconds)) - elapsed
		time.Sleep(diff)
		l.send(noteEvent.Message)
	}
}

func (l *LightPlayer) Close() error {
	var err error

	if l.controller != nil {
		err = l.controller.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func (l *LightPlayer) send(message smf.Message) error {
	var channel uint8
	var key uint8
	var velocity uint8

	if message.GetNoteOn(&channel, &key, &velocity) {
		if os.Getenv("DEBUG") != "" {
			fmt.Printf("noteOn (channel=%d, key=%d, velocity=%d)\n", channel, key, velocity)
		}

		effectString, ok := l.mapping.Channels[channel]
		if ok {
			effect := stringToEffect(effectString, velocity)
			note := noteName(key)
			zoneName, ok := l.mapping.Keys[note]

			effect = l.noteStatuses.Play(key, channel, effect)

			if os.Getenv("DEBUG") != "" {
				fmt.Printf("  note=%s, zone=%s, effect=%8x\n", note, zoneName, effect)
			}

			if ok && l.controller != nil {
				err := l.controller.SetZoneEffect(zoneName, effect)
				if err != nil {
					fmt.Println(err)
					return err
				}
			}
		}
	} else if message.GetNoteOff(&channel, &key, &velocity) {
		if os.Getenv("DEBUG") != "" {
			fmt.Printf("noteOff (channel=%d, key=%d, velocity=%d)\n", channel, key, velocity)
		}

		note := noteName(key)
		zoneName, ok := l.mapping.Keys[note]

		effect := l.noteStatuses.Release(key, channel)

		if os.Getenv("DEBUG") != "" {
			fmt.Printf("  note=%s, zone=%s, effect=%8x\n", note, zoneName, effect)
		}

		if ok && l.controller != nil {
			err := l.controller.SetZoneEffect(zoneName, effect)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func stringToEffect(s string, velocity uint8) uint32 {
	if strings.HasPrefix(s, "0x") {
		colorInt64, err := strconv.ParseInt(s[2:], 16, 32)
		if err != nil {
			return 0
		}
		color := uint32(colorInt64)

		// Multiply RGB color values by velocity
		color = (((color & 0xFF0000) * uint32(velocity) >> 7) & 0xFF0000) |
			(((color & 0x00FF00) * uint32(velocity) >> 7) & 0x00FF00) |
			(((color & 0x0000FF) * uint32(velocity) >> 7) & 0x0000FF)

		return color
	}

	return 0
}

var noteNames = [12]string{
	"A",
	"A#",
	"B",
	"C",
	"C#",
	"D",
	"D#",
	"E",
	"F",
	"F#",
	"G",
	"G#",
}

func noteName(key uint8) string {
	if key < 21 {
		return ""
	}

	subOctave := (key - 21) % 12
	octave := (key - 12) / 12

	return noteNames[subOctave] + strconv.Itoa(int(octave))
}
