package main

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gopxl/beep"
	"github.com/gopxl/beep/mp3"
	"github.com/gopxl/beep/speaker"
)

func main() {
	for {
		mp3Files, midiFiles := listFiles()

		for _, i := range rand.Perm(len(mp3Files)) {
			mp3File := mp3Files[i]
			midiFile := strings.TrimSuffix(mp3File, "mp3") + "mid"

			if _, ok := midiFiles[midiFile]; !ok {
				midiFile = ""
			}

			fmt.Printf("Playing file %s (%s)\n", mp3File, midiFile)
			err := playFile(mp3File, midiFile)
			if err != nil {
				fmt.Println(err)
				time.Sleep(1 * time.Second)
			}
		}
	}
}

func listFiles() ([]string, map[string]struct{}) {
	files, err := os.ReadDir(".")
	if err != nil {
		panic(err)
	}

	mp3Files := []string{}
	midiFiles := map[string]struct{}{}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if strings.HasSuffix(file.Name(), ".mp3") {
			mp3Files = append(mp3Files, file.Name())
		} else if strings.HasSuffix(file.Name(), ".mid") {
			midiFiles[file.Name()] = struct{}{}
		}
	}

	sort.Strings(mp3Files)

	return mp3Files, midiFiles
}

func playFile(mp3FileName, midiFileName string) error {
	mp3File, err := os.Open(mp3FileName)
	if err != nil {
		return fmt.Errorf("error opening mp3 file: %w", err)
	}

	mp3Streamer, mp3Format, err := mp3.Decode(mp3File)
	if err != nil {
		return fmt.Errorf("error decoding mp3 file: %w", err)
	}
	defer mp3Streamer.Close()

	speaker.Init(mp3Format.SampleRate, 2048)

	if midiFileName != "" {
		lightPlayer, err := loadLightsForFile(midiFileName)

		if err == nil {
			defer lightPlayer.Close()
			go func() {
				// time.Sleep(10 * time.Millisecond)
				lightPlayer.Play()
			}()
		} else {
			return fmt.Errorf("error loading lights for file: %w", err)
		}
	}

	done := make(chan bool)
	speaker.Play(beep.Seq(mp3Streamer, beep.Callback(func() {
		done <- true
	})))

	<-done

	return nil
}

func loadLightsForFile(midiFileName string) (*LightPlayer, error) {
	controller, err := NewLEDController("/dev/ttyACM0")
	if err != nil {
		return nil, fmt.Errorf("error initiating LED controller: %w", err)
	}

	config, err := LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("error loading LED config: %w", err)
	}

	err = controller.Configure(config)
	if err != nil {
		return nil, fmt.Errorf("error configuring LEDs: %w", err)
	}

	player, err := LoadLightPlayer(midiFileName, controller)
	// player, err := LoadLightPlayer(midiFileName, nil)
	if err != nil {
		return nil, fmt.Errorf("error loading light player: %w", err)
	}

	return player, nil
}
