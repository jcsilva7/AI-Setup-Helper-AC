package main

import (
	"ai-setup-helper-backend/internal"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
)

// DisableMut Mutex to handle the above bool value change
var DisableMut sync.Mutex

const providerInstructions = `
## Task
You are an expert Assetto Corsa race engineer generating car setups.
- If "oversteer" or "understeer" are true, adjust the setup to reduce the one that is true.
- If both are false, do not apply any oversteer/understeer-specific correction; base the setup purely on the other provided data (track, car, temps, weather, fuel).
Assume the current setup is only a starting point, not an optimized setup. Analyze every adjustable parameter and modify any parameter that would improve the setup for the given track and conditions. Leave a parameter unchanged only if you determine it is already near its optimal value.
You should return a modified setup. That setup should be a base stable setup, target a predictable, confidence-inspiring setup suitable for most drivers rather than an aggressive qualifying setup.

## Input
You will be given a JSON object with car, track, and condition data. Ignore any field that is nil/null. 

## Output format (strict)
Respond ONLY with a JSON array of {"n":..., "v":...} objects, one per field you are changing, where n is name and v is its new value.
Only modify setup parameters that already exist in the provided setup. Never invent new parameter names.
Stay within each field's min/max range if provided.

## Example Output (format only - not real values):
[{"n":"FRONT_ARB","v":4},{"n":"REAR_TOE","v":-0.15}]

Your entire response must be ONLY the JSON array — nothing before it, nothing after it, no markdown formatting. The first character must be [ and the last character must be ].
`

const openRouterModel = "nvidia/nemotron-3-ultra-550b-a55b:free"

var httpClient = &http.Client{
	// enough time even for the slow responding models (deepseek is quite slow)
	Timeout: 60 * time.Second,
}

type RequestResponse struct {
	Setup      string
	StatusCode *int
	Err        error
}

type RequestFirstHandle struct {
	CachedSetup []byte
	BodyString  string
	StatusCode  *int
	Err         error
	CheckKey    internal.Key
	Comparison  internal.ShadyComparison
}

func handleRequest(req *http.Request) RequestFirstHandle {
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("Error closing body: %v\n", err)
		}
	}(req.Body)

	// Read body
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.Printf("Error reading request body: %v\n", err)

		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return RequestFirstHandle{
				StatusCode: new(http.StatusRequestEntityTooLarge),
				Err:        fmt.Errorf("request body too large: %v", err),
			}
		}

		return RequestFirstHandle{
			StatusCode: new(http.StatusBadRequest),
			Err:        fmt.Errorf("failed to read request: %v", err),
		}
	}

	// read request body
	bodyString := string(bodyBytes)

	// Log body to find funny people
	log.Printf("Request body from %s\n%s", clientIP(req), bodyString)

	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	var body SetupRequest

	// Even the key data is malformed :|
	if err := decoder.Decode(&body); err != nil {
		return RequestFirstHandle{
			StatusCode: new(http.StatusBadRequest),
			Err:        fmt.Errorf("failed to parse request body: %v", err),
		}
	}

	// get key and comparison data
	checkKey := body.Key
	comparison := body.ShadyComparison

	if comparison.TrackTemp == nil {
		comparison.TrackTemp = new(-1.0)
	}
	if comparison.AirTemp == nil {
		comparison.AirTemp = new(-1.0)
	}

	// try to get cached setup
	cachedSetup := SetupCache.GetCacheSetup(checkKey, comparison)
	if cachedSetup != nil {
		return RequestFirstHandle{
			CachedSetup: cachedSetup,
			Err:         nil,
		}
	}

	return RequestFirstHandle{
		CachedSetup: nil,
		BodyString:  bodyString,
		CheckKey:    checkKey,
		Comparison:  comparison,
		Err:         nil,
	}
}

// OpenRouterRequest Makes the request to OpenRouter
func OpenRouterRequest(ctx context.Context, body string) RequestResponse {
	url := "https://openrouter.ai/api/v1/chat/completions"

	payload, err := json.Marshal(map[string]any{
		"model": openRouterModel,
		"messages": []map[string]string{
			{"role": "system", "content": providerInstructions},
			{"role": "user", "content": body},
		},
		"max_tokens":  1500,
		"temperature": 0.2,
	})
	if err != nil {
		return RequestResponse{
			StatusCode: new(http.StatusInternalServerError),
			Err:        fmt.Errorf("error encoding provider payload to OpenRouter: %v", err),
		}
	}

	providerReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return RequestResponse{
			StatusCode: new(http.StatusInternalServerError),
			Err:        fmt.Errorf("error creating request to OpenRouter: %v", err),
		}
	}

	providerReq.Header.Set("Content-Type", "application/json")
	providerReq.Header.Set("Authorization", "Bearer "+apiKey)

	// send request
	resp, err := httpClient.Do(providerReq)
	if err != nil {
		return RequestResponse{
			StatusCode: new(http.StatusInternalServerError),
			Err:        fmt.Errorf("error sending request to OpenRouter: %v", err),
		}
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("Error closing body: %v\n", err)
		}
	}(resp.Body)

	// read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return RequestResponse{
			StatusCode: new(http.StatusInternalServerError),
			Err:        fmt.Errorf("error reading response from OpenRouter: %v", err),
		}
	}

	// check status code from the provider
	if resp.StatusCode != http.StatusOK {
		log.Printf("Provider returned status %d: %s\n", resp.StatusCode, responseBody)
		if resp.StatusCode == http.StatusTooManyRequests {
			return RequestResponse{
				StatusCode: new(http.StatusTooManyRequests),
				Err:        fmt.Errorf("provider rate limited the app: %s", responseBody),
			}
		}

		return RequestResponse{
			StatusCode: new(resp.StatusCode),
			Err:        fmt.Errorf("provider responded with status %d: %s", resp.StatusCode, responseBody),
		}
	}

	var providerResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	// get actual content of the provider response
	if err = json.Unmarshal(responseBody, &providerResp); err != nil {
		return RequestResponse{
			StatusCode: new(http.StatusInternalServerError),
			Err:        fmt.Errorf("error decoding OpenRouter provider response: %v", err),
		}
	}

	// Provider returned nothing
	if len(providerResp.Choices) == 0 {
		return RequestResponse{
			StatusCode: new(http.StatusBadGateway),
			Err:        fmt.Errorf("provider returned no responses"),
		}
	}

	return RequestResponse{
		Setup: providerResp.Choices[0].Message.Content,
		Err:   nil,
	}
}

func AzureRequest(ctx context.Context, body string) RequestResponse {
	client := openai.NewClient(
		option.WithBaseURL(endpoint),
		option.WithHeader("api-key", azureApiKey),
	)

	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: deploymentName,
		Messages: []openai.ChatCompletionMessageParamUnion{
			{
				OfSystem: &openai.ChatCompletionSystemMessageParam{
					Content: openai.ChatCompletionSystemMessageParamContentUnion{
						OfString: openai.String(providerInstructions),
					},
				},
			},
			{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{
						OfString: openai.String(body),
					},
				},
			},
		},
		Temperature:         openai.Float(0.2),
		MaxCompletionTokens: openai.Int(2500),
	})

	if err != nil {
		if apiErr, ok := errors.AsType[*openai.Error](err); ok {
			return RequestResponse{
				StatusCode: new(apiErr.StatusCode),
				Err:        fmt.Errorf("error sending request to Microslop Azure: %v", err),
			}
		}

		return RequestResponse{
			StatusCode: new(http.StatusInternalServerError),
			Err:        fmt.Errorf("error sending request to Microslop Azure: %v", err),
		}
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return RequestResponse{
			StatusCode: new(http.StatusBadGateway),
			Err:        fmt.Errorf("provider returned no responses"),
		}
	}

	return RequestResponse{
		Setup: resp.Choices[0].Message.Content,
		Err:   nil,
	}
}

// DisableOpenRouter Disable OpenRouter when daily limit is reached, and set timer to turn on again at 00:00 UTC
func DisableOpenRouter() {
	DisableMut.Lock()
	defer DisableMut.Unlock()

	// Already disabled
	if !OpenRouterAsProvider.Load() {
		return
	}

	OpenRouterAsProvider.Store(false)

	now := time.Now().UTC()
	nextReset := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	wait := nextReset.Sub(now)

	log.Printf("OpenRouter disabled after daily limit was reached. Turning on in %v", wait)

	time.AfterFunc(wait, func() {
		OpenRouterAsProvider.Store(true)
		log.Println("Openrouter turned on again.")
	})
}
