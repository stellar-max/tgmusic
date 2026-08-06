/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package config

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const cookiesDr = "src/cookies"

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// fetchContent downloads cookies from Pastebin, Batbin, or any direct TXT/text URL.
func fetchContent(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty cookie url")
	}

	rawURL := normalizeCookieURL(raw)

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for %s: %w", rawURL, err)
	}

	req.Header.Set("User-Agent", "TgMusicBot/1.0")
	req.Header.Set("Accept", "text/plain,text/*,*/*")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to GET %s: %w", rawURL, err)
	}
	defer func(body io.ReadCloser) {
		_ = body.Close()
	}(resp.Body)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("unexpected status %d for %s", resp.StatusCode, rawURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read body from %s: %w", rawURL, err)
	}

	content := strings.TrimSpace(string(body))
	if content == "" {
		return "", fmt.Errorf("empty response from %s", rawURL)
	}

	return content, nil
}

// normalizeCookieURL converts supported paste URLs to raw URLs.
// Direct text/txt URLs are returned unchanged.
func normalizeCookieURL(raw string) string {
	trimmed := strings.Trim(raw, "/")
	parts := strings.Split(trimmed, "/")
	id := parts[len(parts)-1]

	if strings.Contains(raw, "pastebin.com") && !strings.Contains(raw, "/raw/") {
		return fmt.Sprintf("https://pastebin.com/raw/%s", id)
	}

	if strings.Contains(raw, "batbin.me") && !strings.Contains(raw, "/raw/") {
		return fmt.Sprintf("https://batbin.me/raw/%s", id)
	}

	return raw
}

// saveContent saves cookie text into src/cookies and returns the file path.
func saveContent(rawURL, content string) (string, error) {
	if err := os.MkdirAll(cookiesDr, 0755); err != nil {
		return "", fmt.Errorf("failed to create cookies dir %s: %w", cookiesDr, err)
	}

	filename := cookieFilename(rawURL)
	filePath := filepath.Join(cookiesDr, filename)

	f, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file %s: %w", filePath, err)
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	if _, err := f.WriteString(content); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return filePath, nil
}

// cookieFilename creates a safe filename from URL.
// Keeps .txt only once if URL already points to a txt file.
func cookieFilename(raw string) string {
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Path != "" {
		base := filepath.Base(parsed.Path)
		base = strings.TrimSpace(base)

		if base != "." && base != "/" && base != "" {
			base = sanitizeFilename(base)

			if strings.EqualFold(filepath.Ext(base), ".txt") {
				return base
			}

			return base + ".txt"
		}
	}

	fallback := strings.NewReplacer(
		"https://", "",
		"http://", "",
		"/", "_",
		"?", "_",
		"&", "_",
		"=", "_",
		"#", "_",
		":", "_",
	).Replace(raw)

	fallback = sanitizeFilename(fallback)
	if fallback == "" {
		fallback = fmt.Sprintf("cookies_%d", time.Now().UnixNano())
	}

	if !strings.EqualFold(filepath.Ext(fallback), ".txt") {
		fallback += ".txt"
	}

	return fallback
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, ":", "_")
	name = strings.ReplaceAll(name, "*", "_")
	name = strings.ReplaceAll(name, "?", "_")
	name = strings.ReplaceAll(name, "\"", "_")
	name = strings.ReplaceAll(name, "<", "_")
	name = strings.ReplaceAll(name, ">", "_")
	name = strings.ReplaceAll(name, "|", "_")
	name = strings.Trim(name, ".")

	return name
}

// saveAllCookies downloads all URLs and stores paths in Conf.CookiesPath.
func saveAllCookies(urls []string) {
	for _, rawURL := range urls {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}

		content, err := fetchContent(rawURL)
		if err != nil {
			slog.Info("Error fetching cookies from", "url", rawURL, "error", err)
			continue
		}

		path, err := saveContent(rawURL, content)
		if err != nil {
			slog.Info("Error saving cookies for", "url", rawURL, "error", err)
			continue
		}

		Conf.CookiesPath = append(Conf.CookiesPath, path)
	}
}
