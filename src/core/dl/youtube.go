/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package dl

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ashokshau/tgmusic/config"
	"ashokshau/tgmusic/src/utils"
)

type youTubeData struct {
	Query    string
	ApiUrl   string
	APIKey   string
	Patterns map[string]*regexp.Regexp
}

type youtubeDownloadResponse struct {
	Title       string `json:"title"`
	VideoID     string `json:"video_id"`
	Format      string `json:"format"`
	Quality     string `json:"quality"`
	DownloadURL string `json:"download_url"`
	Source      string `json:"source"`
}

var youtubePatterns = map[string]*regexp.Regexp{
	"youtube":   regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?youtube\.com/.*`),
	"youtu_be":  regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?youtu\.be/.*`),
	"yt_music":  regexp.MustCompile(`(?i)^(?:https?://)?music\.youtube\.com/.*`),
	"yt_shorts": regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?youtube\.com/shorts/.*`),
}

func newYouTubeData(query string) *youTubeData {
	return &youTubeData{
		Query:    strings.TrimSpace(query),
		ApiUrl:   strings.TrimRight(strings.TrimSpace(config.ApiUrl), "/"),
		APIKey:   strings.TrimSpace(config.ApiKey),
		Patterns: youtubePatterns,
	}
}

func (y *youTubeData) isValid() bool {
	if y.Query == "" {
		slog.Info("The query or patterns are empty.")
		return false
	}

	for _, pattern := range y.Patterns {
		if pattern.MatchString(y.Query) {
			return true
		}
	}

	return false
}

func (y *youTubeData) getInfo() (utils.PlatformTracks, error) {
	if !y.isValid() {
		return utils.PlatformTracks{}, errors.New(
			"the provided URL is invalid or the platform is not supported",
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()

	y.Query = normalizeYouTubeURL(y.Query)

	videoID := extractVideoID(y.Query)
	playlistID := extractPlaylistID(y.Query)

	switch {
	case playlistID != "":
		if strings.HasPrefix(playlistID, "RD") {
			return GetYouTubeMixPlaylist(ctx, playlistID)
		}

		return getYouTubePlaylist(ctx, playlistID)

	case videoID != "":
		for _, query := range []string{videoID, y.Query} {
			tracks, err := searchYouTube(query, 10)
			if err != nil {
				continue
			}

			for _, track := range tracks {
				if track.Id == videoID {
					return utils.PlatformTracks{
						Results: []utils.MusicTrack{track},
					}, nil
				}
			}
		}

		title, err := getYouTubeTitleFromOEmbed(videoID)
		if err == nil && title != "" {
			tracks, searchErr := searchYouTube(title, 10)
			if searchErr == nil {
				for _, track := range tracks {
					if track.Id == videoID {
						return utils.PlatformTracks{
							Results: []utils.MusicTrack{track},
						}, nil
					}
				}
			}
		}

		slog.Warn(
			"Video ID was extracted but no matching track was found in search results",
			"video_id", videoID,
		)

		return getYouTubeVideo(ctx, videoID)
	}

	return utils.PlatformTracks{}, errors.New(
		"no video or playlist results were found",
	)
}

func (y *youTubeData) search() (utils.PlatformTracks, error) {
	tracks, err := searchYouTube(y.Query, 5)
	if err != nil {
		return utils.PlatformTracks{}, err
	}

	if len(tracks) == 0 {
		return utils.PlatformTracks{}, errors.New(
			"no video results were found",
		)
	}

	return utils.PlatformTracks{
		Results: tracks,
	}, nil
}

func (y *youTubeData) getTrack() (utils.TrackInfo, error) {
	if y.Query == "" {
		return utils.TrackInfo{}, errors.New("the query is empty")
	}

	if !y.isValid() {
		return utils.TrackInfo{}, errors.New(
			"the provided URL is invalid or the platform is not supported",
		)
	}

	if y.ApiUrl != "" && y.APIKey != "" {
		trackInfo, err := newApiData(y.Query).getTrack()
		if err == nil {
			return trackInfo, nil
		}
	}

	info, err := y.getInfo()
	if err != nil {
		return utils.TrackInfo{}, err
	}

	if len(info.Results) == 0 {
		return utils.TrackInfo{}, errors.New(
			"no video results were found",
		)
	}

	track := info.Results[0]

	return utils.TrackInfo{
		Id:       track.Id,
		URL:      track.Url,
		Platform: utils.YouTube,
	}, nil
}

func (y *youTubeData) downloadTrack(
	info utils.TrackInfo,
	video bool,
) (string, error) {
	videoID := strings.TrimSpace(info.Id)

	if videoID == "" {
		videoID = extractVideoID(info.URL)
	}

	if videoID == "" {
		videoID = extractVideoID(y.Query)
	}

	if videoID == "" {
		return "", errors.New("youtube video ID is empty")
	}

	if y.ApiUrl != "" {
		filePath, err := y.downloadWithApi(videoID, video)
		if err == nil {
			return filePath, nil
		}

		slog.Warn(
			"YouTube download API failed, falling back to yt-dlp",
			"video_id", videoID,
			"video", video,
			"error", err,
		)
	}

	if !video && strings.TrimSpace(info.CdnURL) != "" {
		return info.CdnURL, nil
	}

	return y.downloadWithYtDlp(videoID, video)
}

func (y *youTubeData) buildYtdlpParams(
	videoID string,
	video bool,
) ([]string, string) {
	outputTemplate := filepath.Join(
		config.DownloadsDir,
		"%(id)s.%(ext)s",
	)

	params := []string{
		"yt-dlp",
		"--no-warnings",
		"--quiet",
		"--geo-bypass",
		"--retries", "2",
		"--continue",
		"--no-part",
		"--concurrent-fragments", "3",
		"--socket-timeout", "10",
		"--throttled-rate", "100K",
		"--retry-sleep", "1",
		"--no-write-thumbnail",
		"--no-write-info-json",
		"--no-embed-metadata",
		"--no-embed-chapters",
		"--no-embed-subs",
		"--extractor-args", "youtube:player_js_version=actual",
		"-o", outputTemplate,
	}

	if video {
		params = append(
			params,
			"-f",
			"bestvideo[height<=720]+bestaudio/best[height<=720]",
			"--merge-output-format",
			"mp4",
		)
	} else {
		params = append(
			params,
			"-f",
			"bestaudio[ext=m4a]/bestaudio",
		)
	}

	cookieFile := y.getCookieFile()

	if cookieFile != "" {
		params = append(
			params,
			"--cookies",
			cookieFile,
		)
	} else if config.Proxy != "" {
		params = append(
			params,
			"--proxy",
			config.Proxy,
		)
	}

	videoURL := "https://www.youtube.com/watch?v=" + videoID

	params = append(
		params,
		videoURL,
		"--print",
		"after_move:filepath",
	)

	return params, cookieFile
}

func (y *youTubeData) downloadWithYtDlp(
	videoID string,
	video bool,
) (string, error) {
	if videoID == "" {
		return "", errors.New("videoID is empty")
	}

	params, cookieFile := y.buildYtdlpParams(videoID, video)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		2*time.Minute,
	)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		params[0],
		params[1:]...,
	)

	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError

		if errors.As(err, &exitErr) {
			stderr := string(exitErr.Stderr)

			if cookieFile != "" &&
				strings.Contains(
					stderr,
					"Sign in to confirm you're not a bot",
				) {
				_ = os.Remove(cookieFile)
			}

			return "", fmt.Errorf(
				"yt-dlp failed with exit code %d: %s",
				exitErr.ExitCode(),
				stderr,
			)
		}

		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf(
				"yt-dlp timed out for video ID: %s",
				videoID,
			)
		}

		return "", fmt.Errorf(
			"an unexpected error occurred while downloading %s: %w",
			videoID,
			err,
		)
	}

	downloadedPath := strings.TrimSpace(string(output))

	if downloadedPath == "" {
		return "", fmt.Errorf(
			"no output path was returned for %s",
			videoID,
		)
	}

	if _, err := os.Stat(downloadedPath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf(
				"the file was not found at the reported path: %s",
				downloadedPath,
			)
		}

		return "", fmt.Errorf(
			"failed to verify downloaded file %s: %w",
			downloadedPath,
			err,
		)
	}

	return downloadedPath, nil
}

func (y *youTubeData) getCookieFile() string {
	cookiesPath := config.CookiesPath

	if len(cookiesPath) == 0 {
		return ""
	}

	n, err := rand.Int(
		rand.Reader,
		big.NewInt(int64(len(cookiesPath))),
	)
	if err != nil {
		slog.Info(
			"Could not generate a random number",
			"error", err,
		)

		return cookiesPath[0]
	}

	return cookiesPath[n.Int64()]
}

func (y *youTubeData) downloadWithApi(
	videoID string,
	video bool,
) (string, error) {
	videoID = strings.TrimSpace(videoID)

	if videoID == "" {
		return "", errors.New("videoID is empty")
	}

	if y.ApiUrl == "" {
		return "", errors.New(
			"youtube download API URL is not configured",
		)
	}

	values := url.Values{
		"id":   {videoID},
		"site": {"ytmp3"},
	}

	expectedFormat := "mp3"
	fileExtension := ".mp3"

	if video {
		expectedFormat = "mp4"
		fileExtension = ".mp4"

		values.Set("format", "mp4")
	} else {
		values.Set("format", "mp3")
		values.Set("quality", "320")
	}

	fullURL := fmt.Sprintf(
		"%s/api/download?%s",
		y.ApiUrl,
		values.Encode(),
	)

	var headers map[string]string

	if y.APIKey != "" {
		headers = map[string]string{
			"X-API-Key": y.APIKey,
		}
	}

	resp, err := sendRequest(
		http.MethodGet,
		fullURL,
		nil,
		headers,
	)
	if err != nil {
		return "", fmt.Errorf(
			"youtube download API request failed: %w",
			err,
		)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(
			io.LimitReader(resp.Body, 2048),
		)

		message := strings.TrimSpace(string(responseBody))
		if message == "" {
			message = resp.Status
		}

		return "", fmt.Errorf(
			"youtube download API returned %s: %s",
			resp.Status,
			message,
		)
	}

	var data youtubeDownloadResponse

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf(
			"failed to decode youtube download API response: %w",
			err,
		)
	}

	data.DownloadURL = strings.TrimSpace(data.DownloadURL)

	if data.DownloadURL == "" {
		return "", errors.New(
			"youtube download API returned an empty download URL",
		)
	}

	parsedURL, err := url.Parse(data.DownloadURL)
	if err != nil ||
		parsedURL.Host == "" ||
		(parsedURL.Scheme != "http" &&
			parsedURL.Scheme != "https") {
		return "", errors.New(
			"youtube download API returned an invalid download URL",
		)
	}

	if data.Format != "" &&
		!strings.EqualFold(data.Format, expectedFormat) {
		return "", fmt.Errorf(
			"youtube download API returned format %q instead of %q",
			data.Format,
			expectedFormat,
		)
	}

	filePath := filepath.Join(
		config.DownloadsDir,
		videoID+fileExtension,
	)

	return downloadFile(
		data.DownloadURL,
		filePath,
		false,
	)
}
