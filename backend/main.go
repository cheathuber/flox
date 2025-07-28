package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

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

func init() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Println("Warning: .env file not found or could not be loaded")
	}
	sitesBaseDir = os.Getenv("SITES_BASE_DIR")
	if sitesBaseDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			panic("unable to get working directory")
		}
		sitesBaseDir = filepath.Join(cwd, "sites")
	}
	siteDirFlag := flag.String("sites-dir", "", "Base directory to store site configs")
	flag.Parse()
	if *siteDirFlag != "" {
		sitesBaseDir = *siteDirFlag
	}

	fmt.Printf("Using sites base directory: %s\n", sitesBaseDir)

	// Make sure the directory exists (create if missing)
	if err := os.MkdirAll(sitesBaseDir, 0755); err != nil {
		panic(fmt.Sprintf("failed to create sites base directory: %v", err))
	}
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
	http.HandleFunc("/api/sites/validate-name", validateSiteNameHandler)
	http.HandleFunc("/api/sites", createSiteHandler)
	http.HandleFunc("/api/sections", getSectionsHandler)
	http.HandleFunc("/api/themes", getSectionsHandler)

	http.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK")
	})

	fmt.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}
