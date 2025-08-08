package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/cors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var Version = "dev"

var siteNameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)

var siteNameBlacklist = map[string]struct{}{
	"www":   {},
	"mail":  {},
	"ftp":   {},
	"admin": {},
	"api":   {},
	// Add more reserved names if needed
}

var sitesBaseDir string
var port int

type Config struct {
	Server struct {
		ListenAddress string `mapstructure:"listen_address"`
		Port          int    `mapstructure:"port"`
	} `mapstructure:"server"`
	Sites struct {
		BaseDir string `mapstructure:"base_dir"`
	} `mapstructure:"sites"`
	DNS struct {
		APIRRSets string `mapstructure:"api_rrsets"`
		APIAuth   string `mapstructure:"api_auth"`
		Domain    string `mapstructure:"domain"`
	} `mapstructure:"dns"`
	Database struct {
		AdminPath string `mapstructure:"admin_path"`
	} `mapstructure:"database"`
	Paths struct {
		TemplateDir string `mapstructure:"template_dir"`
		ScriptDir   string `mapstructure:"script_dir"`
	} `mapstructure:"paths"`
}

var config Config

func initViper() {
	viper.SetConfigName("backend")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/flox/")
	viper.AddConfigPath("$HOME/.config/flox/")
	viper.AddConfigPath(".")
	viper.SetEnvPrefix("FLOX")

	viper.AutomaticEnv()

	// Read the configuration file
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			log.Println("Info: Config file not found, relying on environment variables and defaults.")
		} else {
			// Config file was found but another error was produced
			log.Fatalf("Fatal error reading config file: %v", err)
		}
	} else {
		log.Printf("Using config file: %s", viper.ConfigFileUsed())
	}

	// --- Set default values ---
	// These are used if neither the config file nor env vars provide a value
	// Note: The key names must match the structure in your YAML and how Viper sees them.
	// For nested keys like `sites.base_dir` in YAML, use dot notation.
	viper.SetDefault("sites.base_dir", "./sites") // Default for development
	viper.SetDefault("server.port", 0)            // Default to 0 (auto-select) if not specified

	// --- Bind command-line flags ---
	// Define flags
	flag.String("sites-dir", "", "Base directory to store site configs")
	// Bind the flag to a Viper key. The flag name becomes the key if not specified otherwise.
	// This makes the flag value available via viper.GetString("sites-dir")
	// and gives it the highest precedence (after explicit viper.Set calls).
	viper.BindPFlag("sites.dir_flag", pflag.CommandLine.Lookup("sites-dir")) // Use a distinct key

	// Parse command-line flags
	flag.Parse()

	// --- Determine final values using Viper ---
	// Priority order (highest to lowest):
	// 1. Explicit call to viper.Set() (not used here)
	// 2. Flag (bound via BindPFlag)
	// 3. Environment variable (via AutomaticEnv)
	// 4. Config file value (from ReadInConfig)
	// 5. Default value (set via SetDefault)

	// Get sites base directory
	// Check flag first (highest priority for this specific setting)
	flagSitesDir := viper.GetString("sites.dir_flag")
	if flagSitesDir != "" {
		sitesBaseDir = flagSitesDir
	} else {
		// Fall back to config file or env var
		sitesBaseDir = viper.GetString("sites.base_dir")
	}
	// If it's still empty (unlikely due to default), use cwd logic as final fallback
	// (This logic is mostly covered by the default now, but kept for robustness)
	if sitesBaseDir == "" {
		log.Println("Warning: sites.base_dir is empty, using current directory logic.")
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatalf("Fatal: unable to get working directory: %v", err)
		}
		sitesBaseDir = filepath.Join(cwd, "sites")
	}

	// Get server port
	// Check flag first (highest priority for this specific setting)
	// Note: We need to handle the flag specially because it overrides the config/env *and* has a default (0)
	// Viper's normal precedence might not work perfectly here because the flag default is 0,
	// which is a valid value, but also the "use default/auto" value in our logic.
	// Let's handle it explicitly:
	// flagPortStr := pflag.Lookup("port") // Define a port flag if you want
	// Or, if you don't have a dedicated -port flag, rely on SERVER_PORT env var and config file.
	// Let's assume you rely on SERVER_PORT env var and config for port for now,
	// and keep the flag parsing for sites-dir only in initViper for simplicity.
	// You can add a -port flag later if needed using viper.BindPFlag("server.port", ...)

	// Get port from Viper (env var SERVER_PORT or config file server.port)
	// Viper returns the zero value (0 for int) if not found/set.
	// We handle 0 as "use default/auto-select" in main().
	configuredPort := viper.GetInt("server.port")
	if configuredPort > 0 && configuredPort <= 65535 {
		port = configuredPort
	} else if configuredPort == 0 {
		// Zero is treated as "use default/auto-select"
		port = 0
	} else {
		log.Printf("Warning: Invalid server.port %d from config/env, falling back to automatic port selection", configuredPort)
		port = 0 // Default to auto-select
	}

	// --- Ensure the sites directory exists ---
	log.Printf("Using sites base directory: %s", sitesBaseDir)
	if err := os.MkdirAll(sitesBaseDir, 0755); err != nil {
		log.Fatalf("Failed to create sites base directory '%s': %v", sitesBaseDir, err)
	}
}

func init() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Println("Info: .env file not found or could not be loaded")
	}
	initViper()
}

func validateSiteName(siteName string) error {
	siteName = strings.ToLower(siteName)

	if !siteNameRegex.MatchString(siteName) {
		return errors.New("site name must be 1-63 characters, letters, digits, or hyphens; cannot start or end with hyphen")
	}

	if _, forbidden := siteNameBlacklist[siteName]; forbidden {
		return errors.New("site name is reserved or forbidden")
	}

	exists, err := siteExists(siteName)
	if err != nil {
		return fmt.Errorf("error checking site existence: %v", err)
	}
	if exists {
		return errors.New("site name already exists")
	}
	return nil
}

func siteExists(siteName string) (bool, error) {
	path := filepath.Join(sitesBaseDir, siteName)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

type validationRequest struct {
	SiteName string `json:"siteName"`
}

type validationResponse struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

func validateSiteNameHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "Method not allowed")
		return
	}

	var req validationRequest
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	if err := decoder.Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(validationResponse{Valid: false, Error: "Invalid JSON request"})
		return
	}

	err := validateSiteName(req.SiteName)
	resp := validationResponse{}
	if err != nil {
		resp.Valid = false
		resp.Error = err.Error()
	} else {
		resp.Valid = true
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Site creation API request/response
type siteCreationRequest struct {
	SiteName       string   `json:"siteName"`
	Description    string   `json:"description,omitempty"`
	Style          string   `json:"style,omitempty"`
	InitialContent []string `json:"initialContent,omitempty"`
}

type siteCreationResponse struct {
	Success bool   `json:"success"`
	SiteURL string `json:"siteUrl,omitempty"`
	Error   string `json:"error,omitempty"`
}

type SiteConfig struct {
	SiteName       string    `json:"siteName"`
	Description    string    `json:"description,omitempty"`
	Style          string    `json:"style,omitempty"`
	InitialContent []string  `json:"initialContent,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

// Helper for JSON response with Content-Type and encoding
func respondJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func createSiteDir(siteName string) error {
	path := filepath.Join(sitesBaseDir, siteName)
	// Use os.Mkdir with proper mode; will fail if directory exists, which helps atomically lock
	err := os.Mkdir(path, 0755)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("site already exists")
		}
		return err
	}
	return nil
}

func writeSiteConfig(baseDir, siteName string, config SiteConfig) error {
	configPath := filepath.Join(baseDir, siteName, "config.json")
	f, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ") // pretty print JSON with indentation
	return encoder.Encode(config)
}

func createARecord(subdomain, ip string) error {
	apiURL := os.Getenv("DNS_API_RRSETS")
	apiToken := os.Getenv("DNS_API_AUTH")

	if apiURL == "" || apiToken == "" {
		return fmt.Errorf("DNS API config missing")
	}

	// Clean token string (in case of extra quotes)
	apiToken = strings.Trim(apiToken, `"`)

	payload := map[string]interface{}{
		"subname": subdomain,
		"type":    "A",
		"ttl":     3600,
		"records": []string{ip},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %v", err)
	}

	req, err := http.NewRequest("POST", "https://"+apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", apiToken)
	req.Header.Set("Content-Type", "application/json")

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func createSiteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req siteCreationRequest
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "Invalid JSON request", http.StatusBadRequest)
		return
	}

	// Validate site name syntax & blacklist
	if err := validateSiteName(req.SiteName); err != nil {
		respondJSON(w, siteCreationResponse{Success: false, Error: err.Error()})
		return
	}

	// Check if site exists (redundant to mkdir but nicer UX errors)
	exists, err := siteExists(req.SiteName)
	if err != nil {
		log.Printf("error checking site existence: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if exists {
		respondJSON(w, siteCreationResponse{Success: false, Error: "site name already exists"})
		return
	}

	// Now try to create the directory atomically (acts as lock)
	err = createSiteDir(req.SiteName)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			respondJSON(w, siteCreationResponse{Success: false, Error: "site name already exists"})
			return
		}
		log.Printf("error creating site directory: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	config := SiteConfig{
		SiteName:       req.SiteName,
		Description:    req.Description,
		Style:          req.Style,
		InitialContent: req.InitialContent,
		CreatedAt:      time.Now().UTC(),
	}
	if err := writeSiteConfig(sitesBaseDir, req.SiteName, config); err != nil {
		log.Printf("error writing site config: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	siteIP := os.Getenv("SITE_IP")
	if siteIP == "" {
		log.Fatal("SITE_IP is not set in environment")
	}
	err = createARecord(req.SiteName, siteIP)
	if err != nil {
		log.Printf("failed to create DNS A record: %v", err)
		// handle error, maybe rollback or return 500
	}
	// TODO: Initialize site - create config files, provision CMS, create DNS records, etc.

	// Respond with success and constructed site URL
	siteURL := fmt.Sprintf("https://%s.flox.click", req.SiteName)
	respondJSON(w, siteCreationResponse{Success: true, SiteURL: siteURL})
}

func getSectionsHandler(w http.ResponseWriter, r *http.Request) {
	sections := []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Mandatory   bool   `json:"mandatory"`
	}{
		{ID: "header", Name: "Header", Description: "Navigation bar", Mandatory: true},
		{ID: "footer", Name: "Footer", Description: "Impressum and privacy", Mandatory: true},
		{ID: "hero", Name: "Hero Section", Description: "Full-width banner", Mandatory: false},
		{ID: "features", Name: "Features", Description: "Services showcase", Mandatory: false},
		{ID: "testimonials", Name: "Testimonials", Description: "Customer reviews", Mandatory: false},
		{ID: "contact", Name: "Contact Form", Description: "Visitor contact", Mandatory: false},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sections)
}

func getThemesHandler(w http.ResponseWriter, r *http.Request) {
	themes := []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Image string `json:"image,omitempty"`
	}{
		{ID: "light", Name: "Light Theme"},
		{ID: "dark", Name: "Dark Theme"},
		{ID: "material", Name: "Material Design"},
		{ID: "minimal", Name: "Minimalist"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(themes)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sites/validate-name", validateSiteNameHandler)
	mux.HandleFunc("/api/sites", createSiteHandler)
	mux.HandleFunc("/api/sections", getSectionsHandler)
	mux.HandleFunc("/api/themes", getThemesHandler)

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "OK",
			"version": Version,
		})
	})
	c := cors.New(cors.Options{

		AllowedOrigins: []string{
			"https://flox.click",
			"https://www.flox.click",
			"https://app.flox.click",
			"http://localhost:3000", // For local development
			"http://127.0.0.1:3000", // For local development
		},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
		Debug:            true, // Enable for troubleshooting
	})

	var listener net.Listener
	var err error

	if port > 0 {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		listener, err = net.Listen("tcp", addr)
		if err != nil {
			log.Fatalf("Failed to bind to port %d: %v", port, err)
		}
		fmt.Printf("Server is listening on %s\n", addr)
		fmt.Printf("VERSION: %q\n", Version)
	} else {
		// Let OS pick free port
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			log.Fatalf("Failed to listen on a free port: %v", err)
		}
		addr := listener.Addr().(*net.TCPAddr)
		fmt.Printf("Server is listening on 127.0.0.1:%d\n", addr.Port)
	}
	defer listener.Close()

	handler := c.Handler(mux)
	handler = loggingMiddleware(handler)

	if err := http.Serve(listener, handler); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// Simple logging middleware
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL)
		next.ServeHTTP(w, r)
	})
}
