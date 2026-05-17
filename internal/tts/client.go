package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	openRouterKey string
	s3Client      *minio.Client
	bucketName    string
	publicURL     string
	isDirect      bool
}

type TTSResults struct {
	MP3URL string `json:"mp3_url"`
	OGGURL string `json:"ogg_url"`
	WAVURL string `json:"wav_url"`
}

// PcmToFormat uses ffmpeg to transcode raw 16-bit LE mono PCM @ 24kHz to the target format.
func PcmToFormat(pcm []byte, format string) ([]byte, error) {
	args := []string{
		"-f", "s16le",
		"-ar", "24000",
		"-ac", "1",
		"-i", "pipe:0",
		"-f", format,
		"pipe:1",
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdin = bytes.NewReader(pcm)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg (%s): %w: %s", format, err, errOut.String())
	}
	return out.Bytes(), nil
}

func NewClient(openRouterKey, endpoint, accessKey, secretKey, bucketName, publicURL string) (*Client, error) {
	// Trim protocol prefix if present (e.g. from Cloudflare R2 endpoint URL)
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")

	// Use SSL if not localhost or minio container name
	useSSL := !isLocal(endpoint)

	s3Client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("init s3 client: %w", err)
	}

	isDirect := os.Getenv("STORAGE_PUBLIC_URL_IS_DIRECT") == "true"

	return &Client{
		openRouterKey: openRouterKey,
		s3Client:      s3Client,
		bucketName:    bucketName,
		publicURL:     publicURL,
		isDirect:      isDirect,
	}, nil
}

func isLocal(endpoint string) bool {
	return strings.HasPrefix(endpoint, "localhost:") || strings.HasPrefix(endpoint, "minio:") || endpoint == "minio"
}

type Dialogue struct {
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
}

type TTSRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
}

// Map localized speakers to Gemini voices
var voiceMapping = map[string]map[string]string{
	"en": {"Alex": "Zephyr", "Sam": "Puck"},
	"it": {"Marco": "Fenrir", "Sofia": "Kore"},
	"fr": {"Pierre": "Orus", "Marie": "Leda"},
	"bn": {"Fahim": "Charon", "Nusrat": "Aoede"},
}

func getVoiceForSpeaker(lang, speaker string) string {
	if voices, ok := voiceMapping[lang]; ok {
		if voice, exists := voices[speaker]; exists {
			return voice
		}
	}
	return "Zephyr" // Fallback
}

func (c *Client) GenerateAndUpload(ctx context.Context, lang string, dialogues []Dialogue, filename string) (*TTSResults, error) {
	var allPCMData []byte

	// Iterate over each dialogue line and generate audio
	for i, d := range dialogues {
		voice := getVoiceForSpeaker(lang, d.Speaker)
		
		reqBody := TTSRequest{
			Model:          "google/gemini-3.1-flash-tts-preview",
			Input:          d.Text,
			Voice:          voice,
			ResponseFormat: "pcm",
		}

		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("chunk %d marshal: %w", i, err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/audio/speech", bytes.NewBuffer(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("chunk %d req init: %w", i, err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.openRouterKey)

		httpClient := &http.Client{Timeout: 60 * time.Second}
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("chunk %d req do: %w", i, err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("chunk %d openrouter error (%d): %s", i, resp.StatusCode, string(body))
		}

		pcmData, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("chunk %d read: %w", i, err)
		}

		allPCMData = append(allPCMData, pcmData...)
	}

	// Prepare filenames
	baseName := strings.TrimSuffix(filename, ".wav")
	wavName := baseName + ".wav"
	mp3Name := baseName + ".mp3"
	oggName := baseName + ".ogg"

	// Ensure bucket exists
	exists, err := c.s3Client.BucketExists(ctx, c.bucketName)
	if err != nil {
		return nil, err
	}
	if !exists {
		err = c.s3Client.MakeBucket(ctx, c.bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return nil, err
		}
		policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::%s/*"]}]}`, c.bucketName)
		_ = c.s3Client.SetBucketPolicy(ctx, c.bucketName, policy)
	}

	results := &TTSResults{}

	// 1. WAV
	wavData := PcmToWav(allPCMData)
	results.WAVURL, err = c.upload(ctx, wavName, wavData, "audio/wav")
	if err != nil {
		return nil, fmt.Errorf("wav upload: %w", err)
	}

	// 2. MP3
	mp3Data, err := PcmToFormat(allPCMData, "mp3")
	if err != nil {
		return nil, fmt.Errorf("mp3 transcode: %w", err)
	}
	results.MP3URL, err = c.upload(ctx, mp3Name, mp3Data, "audio/mpeg")
	if err != nil {
		return nil, fmt.Errorf("mp3 upload: %w", err)
	}

	// 3. OGG (Opus)
	oggData, err := PcmToFormat(allPCMData, "opus")
	if err != nil {
		return nil, fmt.Errorf("ogg transcode: %w", err)
	}
	results.OGGURL, err = c.upload(ctx, oggName, oggData, "audio/ogg")
	if err != nil {
		return nil, fmt.Errorf("ogg upload: %w", err)
	}

	return results, nil
}

func (c *Client) upload(ctx context.Context, filename string, data []byte, contentType string) (string, error) {
	_, err := c.s3Client.PutObject(ctx, c.bucketName, filename, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", err
	}
	if c.isDirect {
		return fmt.Sprintf("%s/%s", strings.TrimSuffix(c.publicURL, "/"), filename), nil
	}
	return fmt.Sprintf("%s/%s/%s", strings.TrimSuffix(c.publicURL, "/"), c.bucketName, filename), nil
}
