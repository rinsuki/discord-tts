package session

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
)

// TtsSession is a data structure for managing bot agents that participate in one voice channel
type TtsSession struct {
	TextChanelID    string
	VoiceConnection *discordgo.VoiceConnection
	Mut             sync.Mutex
	SpeechSpeed     float32
}

// NewTtsSession create new TtsSession
func NewTtsSession() *TtsSession {
	return &TtsSession{
		TextChanelID:    "",
		VoiceConnection: nil,
		Mut:             sync.Mutex{},
		SpeechSpeed:     1.0,
	}
}

// SendMessage send text to text chat
func (t *TtsSession) SendMessage(discord *discordgo.Session, format string, v ...interface{}) {
	if t.TextChanelID == "" {
		log.Println("Error sending message: TextChanelID is not set")
	}
	msg := fmt.Sprintf(format, v...)
	_, err := discord.ChannelMessageSend(t.TextChanelID, "[BOT] "+msg)
	log.Println(">>> " + msg)
	if err != nil {
		log.Println("Error sending message: ", err)
	}
}

// Speech speech the received text on the voice channel
func (t *TtsSession) Speech(discord *discordgo.Session, text string) error {
	if regexp.MustCompile(`<a:|<@|<#|<@&|http`).MatchString(text) {
		t.SendMessage(discord, "Skipped reading")
		return fmt.Errorf("text is emoji, mention channel, group mention or url")
	}

	lang := "ja"
	if regexp.MustCompile("^[a-zA-Z0-9\\s.,]+$").MatchString(text) {
		lang = "en"
	}

	t.Mut.Lock()
	defer t.Mut.Unlock()

	voiceURL := fmt.Sprintf("http://translate.google.com/translate_tts?ie=UTF-8&textlen=32&client=tw-ob&q=%s&tl=%s", url.QueryEscape(text), lang)
	err := t.playAudioFile(voiceURL)
	if err != nil {
		t.SendMessage(discord, "err=%s", err.Error())
		return fmt.Errorf("t.playAudioFile(voiceURL:%+v) fail: %w", voiceURL, err)
	}
	return nil
}

// playAudioFile play audio file on the voice channel
func (t *TtsSession) playAudioFile(filename string) error {
	if err := t.VoiceConnection.Speaking(true); err != nil {
		return fmt.Errorf("t.VoiceConnection.Speaking(true) fail: %w", err)
	}
	defer func() {
		if err := t.VoiceConnection.Speaking(false); err != nil {
			log.Fatal(err)
		}
	}()

	opts := dca.StdEncodeOptions
	opts.CompressionLevel = 0
	opts.RawOutput = true
	opts.Bitrate = 120
	opts.AudioFilter = fmt.Sprintf("atempo=%f", t.SpeechSpeed)

	encodeSession, err := dca.EncodeFile(filename, opts)
	if err != nil {
		return fmt.Errorf("dca.EncodeFile(filename:%+v, opts:%+v) fail: %w", filename, opts, err)
	}

	done := make(chan error)
	stream := dca.NewStream(encodeSession, t.VoiceConnection, done)
	ticker := time.NewTicker(time.Second)

	for {
		select {
		case err := <-done:
			if err != nil && err != io.EOF {
				return err
			}
			encodeSession.Truncate()
			return nil
		case <-ticker.C:
			stats := encodeSession.Stats()
			playbackPosition := stream.PlaybackPosition()
			log.Printf("Sending Now... : Playback: %10s, Transcode Stats: Time: %5s, Size: %5dkB, Bitrate: %6.2fkB, Speed: %5.1fx\r", playbackPosition, stats.Duration.String(), stats.Size, stats.Bitrate, stats.Speed)
		}
	}
}