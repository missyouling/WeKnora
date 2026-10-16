package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

func loadEnv(path string) map[string]string {
	m := map[string]string{}
	b, _ := os.ReadFile(path)
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		m[strings.TrimSpace(kv[0])] = strings.TrimSpace(strings.Trim(kv[1], `"`))
	}
	return m
}

func newHTTPClient(secretID, secretKey string) *http.Client {
	client := &http.Client{Timeout: 20 * time.Second}
	client.Transport = &cos.AuthorizationTransport{
		SecretID:  secretID,
		SecretKey: secretKey,
		Transport: client.Transport,
	}
	return client
}

func main() {
	env := loadEnv(".env")
	secretID := env["COS_SECRET_ID"]
	secretKey := env["COS_SECRET_KEY"]
	customDomain := env["COS_CUSTOM_DOMAIN"]
	key := "weknora/10000/1f8cee13-2311-4488-a75c-e9b5f78f5f16/1791624673721345200.pdf"

	// 自定义域名直签（与生产 GetFileURL presignClient 一致）
	u, _ := url.Parse("https://" + customDomain + "/")
	client := cos.NewClient(&cos.BaseURL{BucketURL: u}, newHTTPClient(secretID, secretKey))
	opts := &cos.PresignedURLOptions{Query: &url.Values{"response-content-disposition": []string{"inline"}}}
	presigned, err := client.Object.GetPresignedURL(context.Background(), http.MethodGet, key, secretID, secretKey, time.Hour, opts)
	if err != nil {
		fmt.Println("SIGN_ERR:", err)
		return
	}
	ps := presigned.String()
	hostEnd := strings.Index(ps, "/weknora/")
	if hostEnd < 0 {
		hostEnd = strings.Index(ps, "?")
	}
	fmt.Println("SIGNED_PREFIX:", ps[:hostEnd])

	req, _ := http.NewRequest(http.MethodGet, ps, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("GET_ERR:", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 600))
	fmt.Println("STATUS:", resp.StatusCode)
	fmt.Println("BODY:", string(body))
}
