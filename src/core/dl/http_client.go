/*
 * TgMusicBot - Telegram Music Bot
 *  Copyright (c) 2025-2026 Ashok Shau
 *
 *  Licensed under GNU GPL v3
 *  See https://github.com/AshokShau/TgMusicBot
 */

package dl

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"ashokshau/tgmusic/config"
)

const (
	defaultRequestTimeout = 30 * time.Second
	defaultConnectTimeout = 15 * time.Second
	maxRetries            = 2
	initialBackoff        = 1 * time.Second
)

var client = &http.Client{
	Timeout: defaultRequestTimeout,

	Transport: &http.Transport{
		TLSHandshakeTimeout: defaultConnectTimeout,

		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},

		ResponseHeaderTimeout: defaultRequestTimeout,
		ExpectContinueTimeout: 1 * time.Second,

		DialContext: (&net.Dialer{
			Timeout:   defaultConnectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,

		IdleConnTimeout:     90 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     20,
		DisableCompression:  false,
		ForceAttemptHTTP2:   true,
	},

	CheckRedirect: func(
		req *http.Request,
		via []*http.Request,
	) error {
		if len(via) >= 5 {
			return fmt.Errorf(
				"too many redirects (%d)",
				len(via),
			)
		}

		return nil
	},
}

func sendRequest(
	method string,
	fullURL string,
	body io.Reader,
	headers map[string]string,
) (*http.Response, error) {
	var bodyData []byte
	var err error

	if body != nil {
		bodyData, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to read request body: %w",
				err,
			)
		}
	}

	var resp *http.Response
	var reqErr error

	backoff := initialBackoff

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff)
			backoff *= 2
		}

		ctx, cancel := context.WithTimeout(
			context.Background(),
			defaultRequestTimeout,
		)

		var requestBody io.Reader

		if bodyData != nil {
			requestBody = bytes.NewReader(bodyData)
		}

		req, err := http.NewRequestWithContext(
			ctx,
			method,
			fullURL,
			requestBody,
		)
		if err != nil {
			cancel()

			reqErr = fmt.Errorf(
				"failed to create request: %w",
				err,
			)

			break
		}

		req.Header.Set(
			"User-Agent",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) "+
				"AppleWebKit/537.36 (KHTML, like Gecko) "+
				"Chrome/131.0.0.0 Safari/537.36",
		)

		req.Header.Set("Accept", "*/*")

		for key, value := range headers {
			if strings.TrimSpace(value) != "" {
				req.Header.Set(key, value)
			}
		}

		resp, reqErr = client.Do(req)

		if reqErr == nil {
			if resp.StatusCode < http.StatusInternalServerError {
				resp.Body = &cancelOnClose{
					ReadCloser: resp.Body,
					cancel:     cancel,
				}

				return resp, nil
			}

			_, _ = io.Copy(
				io.Discard,
				io.LimitReader(resp.Body, 64*1024),
			)

			_ = resp.Body.Close()
			cancel()

			reqErr = fmt.Errorf(
				"unexpected status code: %d",
				resp.StatusCode,
			)

			continue
		}

		cancel()

		if isTemporaryError(reqErr) {
			slog.Info(
				"Temporary HTTP request error",
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"error", reqErr,
			)

			continue
		}

		break
	}

	if reqErr == nil {
		reqErr = fmt.Errorf(
			"request failed after %d attempts",
			maxRetries,
		)
	}

	return nil, fmt.Errorf(
		"request failed: %s",
		maskSensitiveInfo(reqErr.Error()),
	)
}

func maskSensitiveInfo(msg string) string {
	if strings.TrimSpace(config.ApiKey) == "" {
		return msg
	}

	return strings.ReplaceAll(
		msg,
		config.ApiKey,
		"REDACTED",
	)
}

func isTemporaryError(err error) bool {
	var netErr net.Error

	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	return false
}

func generateUniqueName(ext string) string {
	n, err := rand.Int(
		rand.Reader,
		big.NewInt(99999),
	)
	if err != nil {
		return fmt.Sprintf(
			"%d%s",
			time.Now().UnixNano(),
			ext,
		)
	}

	return fmt.Sprintf(
		"%d_%05d%s",
		time.Now().UnixNano(),
		n.Int64(),
		ext,
	)
}

func determineFilename(
	urlStr string,
	contentDisp string,
) string {
	if filename := extractFilename(contentDisp); filename != "" {
		return filepath.Join(
			config.DownloadsDir,
			sanitizeFilename(filename),
		)
	}

	if parsedURL, err := url.Parse(urlStr); err == nil {
		filename := path.Base(parsedURL.Path)

		if filename != "" &&
			filename != "/" &&
			filename != "." &&
			!strings.Contains(filename, "?") {
			return filepath.Join(
				config.DownloadsDir,
				sanitizeFilename(filename),
			)
		}
	}

	return filepath.Join(
		config.DownloadsDir,
		generateUniqueName(".tmp"),
	)
}

func writeToFile(
	filename string,
	data io.Reader,
) error {
	out, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf(
			"failed to create the file: %w",
			err,
		)
	}

	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, data); err != nil {
		return fmt.Errorf(
			"failed to write to the file: %w",
			err,
		)
	}

	return nil
}

func downloadFile(
	urlStr string,
	fileName string,
	overwrite bool,
) (string, error) {
	if urlStr == "" {
		return "", errors.New(
			"an empty URL was provided",
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		downloadTimeout,
	)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		urlStr,
		nil,
	)
	if err != nil {
		return "", fmt.Errorf(
			"failed to create the request: %w",
			err,
		)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf(
			"the request failed: %w",
			err,
		)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"unexpected status code received: %d",
			resp.StatusCode,
		)
	}

	if fileName == "" {
		fileName = determineFilename(
			urlStr,
			resp.Header.Get("Content-Disposition"),
		)
	}

	if !overwrite {
		if _, err := os.Stat(fileName); err == nil {
			return fileName, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf(
				"failed to inspect existing file: %w",
				err,
			)
		}
	}

	if err := os.MkdirAll(
		filepath.Dir(fileName),
		defaultDownloadDirPerm,
	); err != nil {
		return "", fmt.Errorf(
			"failed to create the directory: %w",
			err,
		)
	}

	tempPath := fileName + ".part"

	if err := writeToFile(tempPath, resp.Body); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}

	if err := os.Rename(tempPath, fileName); err != nil {
		_ = os.Remove(tempPath)

		return "", fmt.Errorf(
			"failed to rename the temporary file: %w",
			err,
		)
	}

	return fileName, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()

	return err
}
