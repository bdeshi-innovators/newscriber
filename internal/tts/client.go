package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
}

func NewClient(openRouterKey, endpoint, accessKey, secretKey, bucketName, publicURL string) (*Client, error) {
	// Use SSL if not localhost or minio container name
	useSSL := !isLocal(endpoint)

	s3Client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("init s3 client: %w", err)
	}

	return &Client{
		openRouterKey: openRouterKey,
		s3Client:      s3Client,
		bucketName:    bucketName,
		publicURL:     publicURL,
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

func (c *Client) GenerateAndUpload(ctx context.Context, lang string, dialogues []Dialogue, filename string) (string, error) {
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
			return "", fmt.Errorf("chunk %d marshal: %w", i, err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/audio/speech", bytes.NewBuffer(jsonBody))
		if err != nil {
			return "", fmt.Errorf("chunk %d req init: %w", i, err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.openRouterKey)

		httpClient := &http.Client{Timeout: 60 * time.Second}
		resp, err := httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("chunk %d req do: %w", i, err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return "", fmt.Errorf("chunk %d openrouter error (%d): %s", i, resp.StatusCode, string(body))
		}

		pcmData, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("chunk %d read: %w", i, err)
		}

		allPCMData = append(allPCMData, pcmData...)
	}

	// Convert the concatenated PCM data to WAV
	wavData := PcmToWav(allPCMData)

	// Upload to S3
	exists, err := c.s3Client.BucketExists(ctx, c.bucketName)
	if err != nil {
		return "", err
	}
	if !exists {
		err = c.s3Client.MakeBucket(ctx, c.bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return "", err
		}
		policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::%s/*"]}]}`, c.bucketName)
		_ = c.s3Client.SetBucketPolicy(ctx, c.bucketName, policy)
	}

	_, err = c.s3Client.PutObject(ctx, c.bucketName, filename, bytes.NewReader(wavData), int64(len(wavData)), minio.PutObjectOptions{
		ContentType: "audio/wav",
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/%s/%s", strings.TrimSuffix(c.publicURL, "/"), c.bucketName, filename), nil
}
