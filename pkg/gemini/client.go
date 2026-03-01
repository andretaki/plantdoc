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
	"time"
)

type Client struct {
	apiKey     string
	httpClient *http.Client
}

type AnalysisResult struct {
	Species        string    `json:"species"`
	CommonName     string    `json:"common_name"`
	HealthScore    int       `json:"health_score"`
	Confidence     string    `json:"confidence"`
	Diagnosis      string    `json:"diagnosis"`
	CareTips       string    `json:"care_tips"`
	SubScores      SubScores `json:"sub_scores"`
	Urgent         string    `json:"urgent,omitempty"`
	SeasonalAdvice string    `json:"seasonal_advice,omitempty"`
}

type SubScores struct {
	Foliage   int `json:"foliage"`
	Hydration int `json:"hydration"`
	PestRisk  int `json:"pest_risk"`
	Vitality  int `json:"vitality"`
}

type BulkResult struct {
	PlantCount int              `json:"plant_count"`
	Plants     []AnalysisResult `json:"plants"`
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 45 * time.Second},
	}
}

const singlePlantPromptTemplate = `You are a world-class botanist and plant pathologist with 30 years of field experience. Analyze this plant photograph with the precision and depth of a professional consultation.

IDENTIFICATION: Examine leaf morphology, venation patterns, growth habit, stem characteristics, and any visible reproductive structures. Provide your best identification with a confidence qualifier.

HEALTH ASSESSMENT: Score each dimension 1-10 (10 = perfect):
- foliage: Leaf color saturation, turgor pressure, margin integrity, chlorosis/necrosis extent
- hydration: Signs of over/under watering — leaf curl, soil moisture visible, root crown condition
- pest_risk: Evidence of insects, fungal bodies, webbing, frass, lesions, or pathogen symptoms
- vitality: Overall vigor — new growth present, internode spacing, stem woodiness, branching pattern

DIAGNOSIS: Describe what you observe like a doctor's clinical notes. Be specific: "lower leaves show interveinal chlorosis suggesting iron deficiency" not "plant looks unhealthy."

CARE TIPS: Prescribe specific interventions. Include exact measurements when relevant: "Water with 200ml every 5 days, reduce to every 8 days in winter" not "water regularly."

URGENT: If the plant faces imminent death or rapid decline, flag it here. Otherwise omit this field.

SEASONAL: Note any time-sensitive care adjustments for the current season context: %s.

Respond ONLY with valid JSON, no markdown, no code fences:
{
  "species": "Genus species",
  "common_name": "Common name",
  "confidence": "high|medium|low",
  "health_score": 7,
  "sub_scores": {
    "foliage": 8,
    "hydration": 6,
    "pest_risk": 9,
    "vitality": 7
  },
  "diagnosis": "Clinical assessment in 3-4 sentences with specific observations.",
  "care_tips": "3-4 specific actionable interventions with quantities and timing.",
  "urgent": "Only if critical — otherwise omit this field entirely.",
  "seasonal_advice": "Spring-specific guidance for this species."
}`

const bulkPlantPromptTemplate = `You are a world-class botanist. This photograph contains MULTIPLE plants. Identify and analyze EACH plant separately.

For each plant, examine leaf morphology, health indicators, and provide a professional-grade assessment.
Align seasonal recommendations to this context: %s.

Score each dimension 1-10 (10 = perfect):
- foliage: Leaf color, turgor, margins, chlorosis/necrosis
- hydration: Over/under watering signs, soil moisture
- pest_risk: Insects, fungal bodies, lesions, pathogen symptoms
- vitality: New growth, internode spacing, overall vigor

Respond ONLY with valid JSON, no markdown, no code fences:
{
  "plant_count": 3,
  "plants": [
    {
      "species": "Genus species",
      "common_name": "Common name",
      "confidence": "high|medium|low",
      "health_score": 7,
      "sub_scores": {
        "foliage": 8,
        "hydration": 6,
        "pest_risk": 9,
        "vitality": 7
      },
      "diagnosis": "Clinical assessment with specific observations.",
      "care_tips": "Specific actionable interventions with quantities.",
      "seasonal_advice": "Spring-specific guidance."
    }
  ]
}

List every distinct plant species visible. If the same species appears in multiple pots, still list each separately with its own health assessment based on its individual condition.`

func (c *Client) AnalyzePlant(ctx context.Context, imageData []byte, mimeType string, previousDiagnosis *string) (*AnalysisResult, error) {
	prompt := fmt.Sprintf(singlePlantPromptTemplate, seasonalContext(time.Now()))

	if previousDiagnosis != nil {
		prompt += fmt.Sprintf("\n\nPREVIOUS ASSESSMENT for longitudinal comparison:\n%s\n\nCompare current state to previous. Note improvements, decline, or stasis in each dimension. Reference specific changes.", *previousDiagnosis)
	}

	text, err := c.callGemini(ctx, imageData, mimeType, prompt)
	if err != nil {
		return nil, err
	}

	var result AnalysisResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("parsing analysis result: %w\nraw text: %s", err, text)
	}

	return &result, nil
}

func (c *Client) AnalyzeBulk(ctx context.Context, imageData []byte, mimeType string) (*BulkResult, error) {
	text, err := c.callGemini(ctx, imageData, mimeType, fmt.Sprintf(bulkPlantPromptTemplate, seasonalContext(time.Now())))
	if err != nil {
		return nil, err
	}

	var result BulkResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		// Maybe it returned a single plant instead of bulk
		var single AnalysisResult
		if err2 := json.Unmarshal([]byte(text), &single); err2 == nil {
			return &BulkResult{PlantCount: 1, Plants: []AnalysisResult{single}}, nil
		}
		return nil, fmt.Errorf("parsing bulk result: %w\nraw text: %s", err, text)
	}

	return &result, nil
}

func (c *Client) callGemini(ctx context.Context, imageData []byte, mimeType string, prompt string) (string, error) {
	b64Image := base64.StdEncoding.EncodeToString(imageData)

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
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s", c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling Gemini API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gemini API error %d: %s", resp.StatusCode, string(body))
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
		return "", fmt.Errorf("parsing Gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from Gemini")
	}

	text := geminiResp.Candidates[0].Content.Parts[0].Text
	jsonText, err := extractJSONPayload(text)
	if err != nil {
		preview := strings.TrimSpace(text)
		if len(preview) > 350 {
			preview = preview[:350] + "..."
		}
		return "", fmt.Errorf("invalid JSON payload from Gemini: %w (preview: %s)", err, preview)
	}

	return jsonText, nil
}

func seasonalContext(now time.Time) string {
	season := "winter"
	switch now.Month() {
	case time.March, time.April, time.May:
		season = "spring"
	case time.June, time.July, time.August:
		season = "summer"
	case time.September, time.October, time.November:
		season = "autumn"
	}
	return fmt.Sprintf("%s (%s in Northern Hemisphere)", now.Format("January 2006"), season)
}

func extractJSONPayload(text string) (string, error) {
	clean := strings.TrimSpace(text)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	if json.Valid([]byte(clean)) {
		return clean, nil
	}

	start := strings.IndexByte(clean, '{')
	end := strings.LastIndexByte(clean, '}')
	if start >= 0 && end > start {
		candidate := strings.TrimSpace(clean[start : end+1])
		if json.Valid([]byte(candidate)) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no valid JSON object found")
}
