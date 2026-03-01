package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	apiKey     string
	httpClient *http.Client
}

type AnalysisResult struct {
	Species     string `json:"species"`
	CommonName  string `json:"common_name"`
	HealthScore int    `json:"health_score"`
	Diagnosis   string `json:"diagnosis"`
	CareTips    string `json:"care_tips"`
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

func (c *Client) AnalyzePlant(ctx context.Context, imageData []byte, mimeType string, previousDiagnosis *string) (*AnalysisResult, error) {
	b64Image := base64.StdEncoding.EncodeToString(imageData)

	prompt := `You are a plant identification and health expert. Analyze this plant photo.

Respond ONLY with valid JSON (no markdown, no code fences, no extra text):
{
  "species": "Scientific name",
  "common_name": "Common name",
  "health_score": 7,
  "diagnosis": "Detailed health assessment in 2-3 sentences.",
  "care_tips": "Specific actionable care advice in 2-3 sentences."
}

health_score is 1-10 where 10 is perfectly healthy.`

	if previousDiagnosis != nil {
		prompt += fmt.Sprintf("\n\nPrevious assessment for comparison: %s\nNote any improvements or decline.", *previousDiagnosis)
	}

	reqBody := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{"text": prompt},
					{
						"inline_data": map[string]any{
							"mime_type": mimeType,
							"data":      b64Image,
						},
					},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling Gemini API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gemini API error %d: %s", resp.StatusCode, string(body))
	}

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("parsing Gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response from Gemini")
	}

	text := geminiResp.Candidates[0].Content.Parts[0].Text
	// Strip markdown code fences if Gemini includes them despite instructions
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var result AnalysisResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("parsing analysis result: %w\nraw text: %s", err, text)
	}

	return &result, nil
}
