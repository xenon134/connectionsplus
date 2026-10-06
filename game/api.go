package game

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type Response struct {
	Status     string     `json:"status"`
	ID         int        `json:"id"`
	PrintDate  string     `json:"print_date"`
	Editor     string     `json:"editor"`
	Categories []Category `json:"categories"`
}

type Category struct {
	Title string `json:"title"`
	Cards []Card `json:"cards"`
}

type Card struct {
	Content  string `json:"content"`
	Position int    `json:"position"`
}

type Group struct {
	Title string
	Index int
}

func fetch(urlString string) ([]byte, error) {
	resp, err := http.Get(urlString)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from URL: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return data, nil
}

func getConnectionsJSON(date time.Time) ([]byte, error) {
	jsonFilename := fmt.Sprintf("%s.json", date.Format("2006-01-02"))

	dataUrl, err := url.JoinPath("https://www.nytimes.com/svc/connections/v2/", jsonFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to join URL: %w", err)
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = filepath.Join(os.Getenv("HOME"), ".cache")
	}

	connectionsCache := filepath.Join(cacheDir, "connections")
	if err := os.MkdirAll(connectionsCache, 0755); err != nil {
		connectionsData, err := fetch(dataUrl)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch data: %w", err)
		}

		return connectionsData, nil
	}

	cacheFile := filepath.Join(connectionsCache, jsonFilename)
	cachedData, err := os.ReadFile(cacheFile)
	if err != nil {
		connectionsData, err := fetch(dataUrl)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch data: %w", err)
		}

		err = os.WriteFile(cacheFile, connectionsData, 0644)
		if err != nil {
			log.Printf("failed to save to cache: %v", err)
		}
		return connectionsData, nil
	}
	return cachedData, nil
}

func parseConnectionsJSON(data []byte) (Response, error) {
	var response Response

	err := json.Unmarshal([]byte(data), &response)
	if err != nil {
		return response, fmt.Errorf("error parsing JSON: %w", err)
	}
	return response, nil
}

// tileEmoji maps a category index to the emoji used in the share string.
func loadPuzzleForDate(d time.Time) error {
	data, err := getConnectionsJSON(d)
	if err != nil {
		return err
	}
	resp, err := parseConnectionsJSON(data)
	if err != nil {
		return err
	}
	currentDate = d
	puzzleResponse = resp
	return nil
}
