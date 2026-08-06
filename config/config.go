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
	"log/slog"
	"os"
	"strconv"
	"strings"

	_ "github.com/joho/godotenv/autoload"
)

var (
	ApiId               = getEnvInt32("API_ID", 0)
	ApiHash             = os.Getenv("API_HASH")
	Token               = os.Getenv("TOKEN")
	DlBotToken          = os.Getenv("BOT_TOKEN")
	SessionStrings      = getSessionStrings("STRING", 10)
	SessionType         = getEnv("SESSION_TYPE", "pyrogram")
	MongoUri            = os.Getenv("MONGO_URI")
	DbName              = getEnv("DB_NAME", "Anon")
	ApiUrl              = getEnv("API_URL", "https://api.onegrab.fun")
	ApiKey              = os.Getenv("API_KEY")
	OwnerId             = getEnvInt64("OWNER_ID", 0)
	LoggerId            = getEnvInt64("LOGGER_ID", 0)
	Proxy               = os.Getenv("PROXY")
	DefaultService      = strings.ToLower(getEnv("DEFAULT_SERVICE", "youtube"))
	MaxFileSize         = getEnvInt64("MAX_FILE_SIZE", 500*1024*1024)
	SongDurationLimit   = getEnvInt64("SONG_DURATION_LIMIT", 17000)
	DownloadsDir        = getEnv("DOWNLOADS_DIR", "database")
	SupportGroup        = getEnv("SUPPORT_GROUP", "https://t.me/BillaCore")
	SupportChannel      = getEnv("SUPPORT_CHANNEL", "https://t.me/BillaSpace")
	StartImg            = getEnv("START_IMG", "https://files.catbox.moe/pnv5r3.jpg")
	Port                = getEnv("PORT", "6060")
	AutoLeave           = getEnvBool("AUTO_LEAVE", true)
	EnableVideoPlayback = getEnvBool("ENABLE_VPLAY", true)

	DEVS        []int64
	CookiesPath []string

	cookiesURL = processCookieURLs(os.Getenv("COOKIES_URL"))
)

func init() {
	loadDevelopers()

	if OwnerId != 0 && !containsInt(DEVS, OwnerId) {
		DEVS = append(DEVS, OwnerId)
	}

	if err := validate(); err != nil {
		slog.Error(
			"Configuration validation failed",
			"error",
			err,
		)
		os.Exit(1)
	}

	if err := os.MkdirAll(DownloadsDir, 0755); err != nil {
		slog.Error(
			"Failed to create downloads directory",
			"directory",
			DownloadsDir,
			"error",
			err,
		)
		os.Exit(1)
	}

	if len(cookiesURL) > 0 {
		if err := os.MkdirAll(cookiesDr, 0750); err != nil {
			slog.Error(
				"Failed to create cookies directory",
				"directory",
				cookiesDr,
				"error",
				err,
			)
			os.Exit(1)
		}

		saveAllCookies(cookiesURL)

		if len(CookiesPath) == 0 {
			slog.Warn("No cookie files were downloaded successfully")
		}
	}
}

func loadDevelopers() {
	devsEnv := os.Getenv("DEVS")
	if devsEnv == "" {
		return
	}

	devsEnv = strings.NewReplacer(
		"\n", " ",
		",", " ",
	).Replace(devsEnv)

	for _, idString := range strings.Fields(devsEnv) {
		idString = strings.TrimSpace(idString)
		if idString == "" {
			continue
		}

		id, err := strconv.ParseInt(idString, 10, 64)
		if err != nil {
			slog.Warn(
				"Invalid developer ID",
				"id",
				idString,
				"error",
				err,
			)
			continue
		}

		if !containsInt(DEVS, id) {
			DEVS = append(DEVS, id)
		}
	}
}

func getEnv(key, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value != "" {
		return value
	}

	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return defaultValue
	}

	return parsed
}

func getEnvInt32(key string, defaultValue int32) int32 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return defaultValue
	}

	return int32(parsed)
}

func getEnvBool(key string, defaultValue bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}

	return parsed
}

func getSessionStrings(prefix string, max int) []string {
	sessions := make([]string, 0, max+1)
	seen := make(map[string]struct{})

	for i := 1; i <= max; i++ {
		key := fmt.Sprintf("%s%d", prefix, i)
		session := strings.TrimSpace(os.Getenv(key))

		if session == "" {
			continue
		}

		if _, exists := seen[session]; exists {
			continue
		}

		seen[session] = struct{}{}
		sessions = append(sessions, session)
	}

	session := strings.TrimSpace(os.Getenv(prefix))
	if session != "" {
		if _, exists := seen[session]; !exists {
			sessions = append(sessions, session)
		}
	}

	return sessions
}

func processCookieURLs(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	urls := make([]string, 0)
	seen := make(map[string]struct{})

	for _, rawURL := range strings.Split(value, ",") {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}

		if _, exists := seen[rawURL]; exists {
			continue
		}

		seen[rawURL] = struct{}{}
		urls = append(urls, rawURL)
	}

	return urls
}

func containsInt(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}

func validate() error {
	required := []struct {
		name  string
		check func() bool
	}{
		{
			name:  "API_ID",
			check: func() bool { return ApiId > 0 },
		},
		{
			name:  "API_HASH",
			check: func() bool { return strings.TrimSpace(ApiHash) != "" },
		},
		{
			name:  "TOKEN",
			check: func() bool { return strings.TrimSpace(Token) != "" },
		},
		{
			name:  "MONGO_URI",
			check: func() bool { return strings.TrimSpace(MongoUri) != "" },
		},
		{
			name:  "OWNER_ID",
			check: func() bool { return OwnerId > 0 },
		},
	}

	missing := make([]string, 0)

	for _, requirement := range required {
		if !requirement.check() {
			missing = append(missing, requirement.name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf(
			"missing required configuration: %s",
			strings.Join(missing, ", "),
		)
	}

	if len(SessionStrings) == 0 {
		return fmt.Errorf(
			"at least one session string (STRING or STRING1-STRING10) is required",
		)
	}

	if !isValidService(DefaultService) {
		slog.Warn(
			"Invalid DEFAULT_SERVICE, using youtube",
			"service",
			DefaultService,
		)
		DefaultService = "youtube"
	}

	return nil
}

func isValidService(service string) bool {
	switch strings.ToLower(strings.TrimSpace(service)) {
	case "youtube", "spotify":
		return true
	default:
		return false
	}
}
