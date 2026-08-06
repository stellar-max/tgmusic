/*
 * TgMusicBot - Telegram Music Bot
 * Copyright (c) 2025-2026 Ashok Shau
 *
 * Licensed under GNU GPL v3
 * See https://github.com/AshokShau/TgMusicBot
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

func fetchContent(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty cookie URL")
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
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf(
			"unexpected status %d for %s",
			resp.StatusCode,
			rawURL,
		)
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

func normalizeCookieURL(raw string) string {
	raw = strings.TrimSpace(raw)

	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	host := strings.ToLower(parsed.Hostname())
	path := strings.Trim(parsed.Path, "/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 {
		return raw
	}

	id := parts[len(parts)-1]
	if id == "" {
		return raw
	}

	switch host {
	case "pastebin.com", "www.pastebin.com":
		if !strings.HasPrefix(parsed.Path, "/raw/") {
			return "https://pastebin.com/raw/" + id
		}

	case "batbin.me", "www.batbin.me":
		if !strings.HasPrefix(parsed.Path, "/raw/") {
			return "https://batbin.me/raw/" + id
		}
	}

	return raw
}

func saveContent(rawURL, content string) (string, error) {
	if err := os.MkdirAll(cookiesDr, 0750); err != nil {
		return "", fmt.Errorf(
			"failed to create cookies directory %s: %w",
			cookiesDr,
			err,
		)
	}

	filename := cookieFilename(rawURL)
	filePath := filepath.Join(cookiesDr, filename)

	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return filePath, nil
}

func cookieFilename(raw string) string {
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Path != "" {
		base := strings.TrimSpace(filepath.Base(parsed.Path))

		if base != "" && base != "." && base != "/" {
			base = sanitizeFilename(base)

			if base != "" {
				if strings.EqualFold(filepath.Ext(base), ".txt") {
					return base
				}

				return base + ".txt"
			}
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

	replacer := strings.NewReplacer(
		"\\", "_",
		"/", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)

	name = replacer.Replace(name)
	name = strings.Trim(name, ". ")

	return name
}

func saveAllCookies(urls []string) {
	CookiesPath = CookiesPath[:0]

	seenPaths := make(map[string]struct{})

	for _, rawURL := range urls {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}

		content, err := fetchContent(rawURL)
		if err != nil {
			slog.Error(
				"Failed to fetch cookies",
				"url",
				rawURL,
				"error",
				err,
			)
			continue
		}

		path, err := saveContent(rawURL, content)
		if err != nil {
			slog.Error(
				"Failed to save cookies",
				"url",
				rawURL,
				"error",
				err,
			)
			continue
		}

		if _, exists := seenPaths[path]; exists {
			continue
		}

		seenPaths[path] = struct{}{}
		CookiesPath = append(CookiesPath, path)

		slog.Info(
			"Cookies saved",
			"url",
			rawURL,
			"path",
			path,
		)
	}
}
