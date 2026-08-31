package main

import (
	"ai-setup-helper-backend/internal"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/joho/godotenv"
)

// SetupCache Cache structure
var SetupCache *internal.Cache

// MachineLimiter
// one for the machine-key-hashes from the CSP sdk, another for the IP addresses
var MachineLimiter *internal.RateLimiter
var IPLimiter *internal.RateLimiter

// DailyLimiter limit daily requests
var DailyLimiter *internal.RateLimiter

// Openrouter key
var apiKey string

// Azure data
var endpoint string
var deploymentName string
var azureApiKey string

// OpenRouterAsProvider Current main provider
var OpenRouterAsProvider atomic.Bool

// 64 (KB)
const maxBodySize int64 = 64

// SetupRequest data to check cache
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

// Strip the response of extra content (some LLMs decide to add ```json[data]```)
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// Check if the setup is a valid json (by unpacking it into a map) and return the map with the data
func validateSetup(responseSetup string) (bool, []map[string]any) {
	// Remove hallucinated extra chars
	cleanContent := stripCodeFence(responseSetup)

	var setupChanges []map[string]any
	if err := json.Unmarshal([]byte(cleanContent), &setupChanges); err != nil {
		log.Printf("Provider content is not a valid JSON array: %v\nResponse: %v\n", err, cleanContent)
		return false, []map[string]any{}
	}

	return true, setupChanges
}

// Make the request to get and return the setup from the LLM
func getSetup(res http.ResponseWriter, req *http.Request) {
	stuff := handleRequest(req)
	if stuff.Err != nil {
		if stuff.StatusCode != nil {
			res.WriteHeader(*stuff.StatusCode)
		} else {
			res.WriteHeader(http.StatusInternalServerError)

		}
		log.Printf("Error reading request info: %v\n\n", stuff.Err)
		return
	}

	if stuff.CachedSetup != nil {
		res.Header().Set("Content-Type", "application/json")
		res.WriteHeader(http.StatusOK)

		if _, err := res.Write(stuff.CachedSetup); err != nil {
			log.Printf("Error writing response: %v\n", err)
			return
		}

		log.Println("Cache hit")
		return
	}

	bodyString := stuff.BodyString

	var setupChanges []map[string]any
	var isSetupValid bool

	// Cache miss, request to provider
	if OpenRouterAsProvider.Load() {
		openRouterResponse := OpenRouterRequest(req.Context(), bodyString)

		isSetupValid, setupChanges = validateSetup(openRouterResponse.Setup)

		if openRouterResponse.Err != nil || !isSetupValid {
			if openRouterResponse.Err == nil {
				openRouterResponse.Err = fmt.Errorf("returned setup was not valid JSON")
			}

			log.Printf("OpenRouter Error -> %s\n", openRouterResponse.Err)

			// Daily limit reached, switch to azure only (and schedule the switch back)
			if openRouterResponse.StatusCode != nil && *openRouterResponse.StatusCode == 429 &&
				strings.Contains(openRouterResponse.Err.Error(), "free-models-per-day") {
				DisableOpenRouter()
			}

			// Try with Microslop alternative (Always fallback to this)
			azureResponse := AzureRequest(req.Context(), bodyString)
			if azureResponse.Err != nil {
				if azureResponse.StatusCode != nil {
					res.WriteHeader(*azureResponse.StatusCode)
				} else {
					res.WriteHeader(http.StatusInternalServerError)
				}

				log.Printf("Azure Error -> %s\n", azureResponse.Err)
				return
			}

			isSetupValid, setupChanges = validateSetup(azureResponse.Setup)
			if !isSetupValid {
				res.WriteHeader(http.StatusBadGateway)
				log.Println("Neither provider returned a valid JSON setup...")
				return
			}

		}
	} else {
		// Straight to azure (OpenRouter daily usage maxed out)
		azureResponse := AzureRequest(req.Context(), bodyString)

		isSetupValid, setupChanges = validateSetup(azureResponse.Setup)

		if azureResponse.Err != nil || !isSetupValid {
			if azureResponse.StatusCode != nil {
				res.WriteHeader(*azureResponse.StatusCode)
			} else {
				res.WriteHeader(http.StatusBadGateway)
			}

			if azureResponse.Err == nil {
				azureResponse.Err = fmt.Errorf("returned setup was not valid JSON")
			}
			log.Printf("Azure Error -> %s\n", azureResponse.Err)
			return
		}
	}

	// Turn the setup into json again to send to the app
	content, err := json.Marshal(setupChanges)
	if err != nil {
		log.Printf("Error encoding setup response: %v\n", err)
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Put in cache
	SetupCache.Put(stuff.CheckKey, stuff.Comparison, content)

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

// Check connectivity endpoint
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

		if !DailyLimiter.Limit(ip) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			log.Println("Daily limit exceeded: ", ip)
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

		// Limit body size read
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize*2048)

		log.Println("Request Received from ", ip)

		next.ServeHTTP(w, r)
	})
}

func main() {
	var err error

	// Only for local development, does not break but unnecessary
	err = godotenv.Load()
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
	ServeMux.Handle("POST /setup", middleware(http.HandlerFunc(getSetup)))

	apiKey = os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("No API key set")
	}

	endpoint = os.Getenv("AZURE_ENDPOINT")
	deploymentName = os.Getenv("AZURE_DEPLOYMENT_NAME")
	azureApiKey = os.Getenv("AZURE_API_KEY")

	if azureApiKey == "" || deploymentName == "" || endpoint == "" {
		log.Fatal("Some of the required info was empty for Microslop Azure.")
	}

	OpenRouterAsProvider.Store(false)

	// Create cache (before server start)
	SetupCache = internal.NewCache(24*time.Hour, 1*time.Hour)

	// Create rate limiter objects
	MachineLimiter = internal.NewRateLimiter(5, 3, 24*time.Hour)
	IPLimiter = internal.NewRateLimiter(15, 5, 24*time.Hour)
	DailyLimiter = internal.NewDailyRateLimiter(50)

	// Load blacklist
	internal.LoadBlacklist("blacklist.txt")

	log.Println("Listening on port " + port)
	err = server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}
