package main

import (
	"ai-setup-helper-backend/internal"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var SetupCache *internal.Cache

// one for the machine-key-hashes from the CSP sdk, another for the IP addresses
var MachineLimiter *internal.RateLimiter
var IPLimiter *internal.RateLimiter

var apiKey string

// 64 (KB)
var maxBodySize int64 = 64

type SetupRequest struct {
	internal.Key
	internal.ShadyComparison
}

// Get client IP (remove port)
func clientIP(req *http.Request) string {
	// check headers because of hosting proxy
	var host string
	// and get first (which should match network ip)
	ips := strings.Split(req.Header.Get("X-Forwarded-For"), ",")
	if len(ips) > 0 {
		host = strings.TrimSpace(ips[0])
	}
	if host != "" {
		return host
	}

	var err error
	host, _, err = net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}

func getSetupRequest(res http.ResponseWriter, req *http.Request) {
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
			http.Error(res, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		http.Error(res, "Failed to read request", http.StatusBadRequest)
		return
	}

	// read request body
	bodyString := string(bodyBytes)
	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	var body SetupRequest

	// Even the key data is malformed :|
	if err := decoder.Decode(&body); err != nil {
		log.Println("Failed to parse request body")
		log.Println(err)

		http.Error(res, "Failed to parse request body", http.StatusBadRequest)
		return
	}

	// get key and comparison data
	checkKey := body.Key
	comparison := body.ShadyComparison

	if comparison.TrackTemp == nil {
		comparison.TrackTemp = new(-1)
	}
	if comparison.AirTemp == nil {
		comparison.AirTemp = new(-1)
	}

	// try to get cached setup
	cachedSetup := SetupCache.GetCacheSetup(checkKey, comparison)
	if cachedSetup != nil {
		res.Header().Set("Content-Type", "application/json")
		res.WriteHeader(http.StatusOK)

		if _, err := res.Write(cachedSetup); err != nil {
			log.Println(err)
			return
		}

		log.Println("Cache Hit")
		return
	}

	// Cache miss, request to provider
	url := "https://openrouter.ai/api/v1/chat/completions"
	model := "google/gemini-3.7-flash"

	providerBodyText := `
You are an expert Assetto Corsa race engineer generating car setups.
You will be given a JSON object with car, track, and condition data. Ignore any field that is nil/null. If there is a value that should be considered into the setup, and is important.
Respond ONLY with a JSON array of {"n":..., "v":...} objects, where n is name and v is value, one per field you are changing.
Only modify setup parameters that already exist in the provided setup. Never invent new parameter names.
Do not include markdown formatting or any text outside the JSON array.
Stay within each field's min/max range if provided.
- If "oversteer" or "understeer" are true, adjust the setup to reduce the one that is true.
- If both are false, do not apply any oversteer/understeer-specific correction; base the setup purely on the other provided data (track, car, temps, weather, fuel).
Assume the current setup is only a starting point, not an optimized setup. Analyze every adjustable parameter and modify any parameter that would improve the setup for the given track and conditions. Leave a parameter unchanged only if you determine it is already near its optimal value.
You should return a modified setup. That setup should be a base stable setup, target a predictable, confidence-inspiring setup suitable for most drivers rather than an aggressive qualifying setup.
Data: ` + bodyString

	payload, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": providerBodyText,
			},
		},
		"max_tokens": 1500,
		"reasoning": map[string]bool{
			"enabled": true,
		},
	})
	if err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		log.Printf("Error encoding provider payload: %v\n", err)
		return
	}

	providerReq, err := http.NewRequestWithContext(req.Context(), "POST", url, bytes.NewBuffer(payload))
	if err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	providerReq.Header.Set("Content-Type", "application/json")
	providerReq.Header.Set("Authorization", "Bearer "+apiKey)

	// send request
	client := &http.Client{
		// enough time even for the slow responding models (deepseek is quite slow)
		Timeout: time.Second * 60,
	}
	resp, err := client.Do(providerReq)
	if err != nil {
		log.Printf("Error sending request: %v\n", err)
		res.WriteHeader(http.StatusInternalServerError)
		return
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
		log.Printf("Error reading response: %v\n", err)
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	// check status code from the provider
	if resp.StatusCode != http.StatusOK {
		log.Printf("Provider returned status %d: %s\n", resp.StatusCode, responseBody)
		if resp.StatusCode == http.StatusTooManyRequests {
			res.WriteHeader(http.StatusTooManyRequests)
		} else {
			res.WriteHeader(http.StatusBadGateway)
		}
		return
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
		log.Printf("Error decoding provider response: %v\n", err)
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Provider returned nothing
	if len(providerResp.Choices) == 0 {
		log.Println("Provider returned no choices")
		res.WriteHeader(http.StatusBadGateway)
		return
	}

	var setupChanges []map[string]any
	if err = json.Unmarshal([]byte(providerResp.Choices[0].Message.Content), &setupChanges); err != nil {
		log.Printf("Provider content is not a valid JSON array: %v\n", err)
		res.WriteHeader(http.StatusBadGateway)
		return
	}

	content, err := json.Marshal(setupChanges)
	if err != nil {
		log.Printf("Error encoding setup response: %v\n", err)
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Put in cache
	SetupCache.Put(checkKey, comparison, content)

	// Write successful response
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusOK)
	_, err = res.Write(content)
	if err != nil {
		log.Println(err)
		return
	}

	log.Println("Request succeeded")
}

// Check connectivity
func healthz(res http.ResponseWriter, _ *http.Request) {
	res.WriteHeader(http.StatusOK)
}

// Rate Limiting and Blacklisting
func middleware(next http.Handler) http.Handler {
	// Worth noting that the security here it's a joke
	// Any script kiddie with an AI and a few braincells can DoS this
	// But the idea of the app is to be plug n' play, and auth would change that
	// If I force people to create accounts (or any other method) to get keys, then people may not be interested enough
	// to use it
	// I would not do all that for an in-game app for a racing game
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)

		// check if IP is blacklisted
		if internal.IsBlacklisted(ip) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			log.Println("Blacklisted IP Requested: ", ip)
			return
		}

		// check if request will be rate limited
		machineHash := r.Header.Get("X-Machine-Hash")
		if machineHash == "" {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			log.Println("Request without machine hash: ", ip)
			return
		}

		if !MachineLimiter.Limit(machineHash) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			log.Println("Rate limit exceeded: ", machineHash, ip)
			return
		}
		if !IPLimiter.Limit(ip) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			log.Println("Rate limit exceeded: ", ip)
			return
		}

		log.Println("Request Received...")

		// Limit body size read
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize*1024)

		next.ServeHTTP(w, r)
	})
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file")
	}

	// Start HTTP server
	ServeMux := http.NewServeMux()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := http.Server{
		Addr:    ":" + port,
		Handler: ServeMux,
	}

	ServeMux.HandleFunc("GET /healthz", healthz)
	ServeMux.Handle("POST /setup", middleware(http.HandlerFunc(getSetupRequest)))

	apiKey = os.Getenv("API_KEY")
	if apiKey == "" {
		log.Println("No API key set")
	}

	// Create cache (before server start)
	SetupCache = internal.NewCache(24*time.Hour, 1*time.Hour)

	// Create rate limiter objects
	MachineLimiter = internal.NewRateLimiter(5, 3, 24*time.Hour)
	IPLimiter = internal.NewRateLimiter(15, 5, 24*time.Hour)

	// Load blacklist
	internal.LoadBlacklist("blacklist.txt")

	log.Println("Listening on port " + port)
	err = server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}
