package dictation

import (
	"context"
	"io"

	"github.com/emiliopalmerini/elevenlabs-go/elevenlabs"
)

const (
	realtimeModel = "scribe_v2_realtime"
	batchModel    = "scribe_v2"
)

type transcriptEvent struct {
	Type  string
	Text  string
	Error string
}

type realtimeTranscript interface {
	SendAudio([]byte, bool) error
	Receive() (transcriptEvent, error)
	Close() error
}

type transcriber interface {
	Connect(context.Context) (realtimeTranscript, error)
	Transcribe(context.Context, string, io.Reader) (string, error)
}

// ElevenLabsTranscriber adapts the SDK to the narrow interface used by the
// dictation handler.
type ElevenLabsTranscriber struct {
	client *elevenlabs.Client
}

func NewElevenLabsTranscriber(client *elevenlabs.Client) *ElevenLabsTranscriber {
	return &ElevenLabsTranscriber{client: client}
}

func (t *ElevenLabsTranscriber) Connect(ctx context.Context) (realtimeTranscript, error) {
	includeTimestamps := false
	noVerbatim := false
	session, err := t.client.STT.ConnectRealtimeTranscript(ctx, elevenlabs.RealtimeTranscriptRequest{
		ModelID:                 realtimeModel,
		AudioFormat:             "pcm_16000",
		LanguageCode:            "it",
		CommitStrategy:          "vad",
		IncludeTimestamps:       &includeTimestamps,
		NoVerbatim:              &noVerbatim,
		MinSpeechDurationMS:     elevenlabs.Ptr(100),
		MinSilenceDurationMS:    elevenlabs.Ptr(300),
		VADSilenceThresholdSecs: elevenlabs.Ptr(1.0),
	})
	if err != nil {
		return nil, err
	}
	return &elevenRealtimeTranscript{session: session}, nil
}

func (t *ElevenLabsTranscriber) Transcribe(ctx context.Context, name string, audio io.Reader) (string, error) {
	transcript, _, err := t.client.STT.CreateTranscript(ctx, elevenlabs.CreateTranscriptRequest{
		ModelID:      batchModel,
		LanguageCode: "it",
		File: &elevenlabs.File{
			Name:   name,
			Reader: audio,
		},
	})
	if err != nil {
		return "", err
	}
	return transcript.Text, nil
}

type elevenRealtimeTranscript struct {
	session *elevenlabs.RealtimeTranscriptSession
}

func (s *elevenRealtimeTranscript) SendAudio(audio []byte, commit bool) error {
	return s.session.SendAudioChunk(elevenlabs.RealtimeAudioChunk{
		Audio: audio, Commit: commit, SampleRate: 16000,
	})
}

func (s *elevenRealtimeTranscript) Receive() (transcriptEvent, error) {
	event, err := s.session.Receive()
	if err != nil {
		return transcriptEvent{}, err
	}
	return transcriptEvent{Type: event.MessageType, Text: event.Text, Error: event.Error}, nil
}

func (s *elevenRealtimeTranscript) Close() error {
	return s.session.Close()
}
