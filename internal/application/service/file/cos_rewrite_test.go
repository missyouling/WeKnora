package file

import (
	"context"
	"strings"
	"testing"
)

// TestPresignCustomDomain verifies that with COS_CUSTOM_DOMAIN set, GetFileURL
// returns a presigned URL whose host is the custom domain and whose signature
// covers that host (q-header-list contains host). Signing against the custom
// domain's Host is the only way COS accepts the signature after the host swap —
// string-replacing the host of a default-domain signature fails with
// SignatureDoesNotMatch.
func TestPresignCustomDomain(t *testing.T) {
	svc, err := newCosClient("weknora-daily-1313344759", "ap-seoul", "AKIDtestsecretid", "testsecretkey")
	if err != nil {
		t.Fatalf("newCosClient failed: %v", err)
	}
	svc.cosPathPrefix = "weknora"
	svc.customDomain = "cos.mozuiapp.com"

	url, err := svc.GetFileURL(context.Background(), "cos://weknora-daily-1313344759/ap-seoul/weknora/10000/abc.pdf")
	if err != nil {
		t.Fatalf("GetFileURL failed: %v", err)
	}
	if !strings.HasPrefix(url, "https://cos.mozuiapp.com/weknora/10000/abc.pdf?") {
		t.Fatalf("expected custom-domain presigned URL, got: %s", url)
	}
	if !strings.Contains(url, "response-content-disposition=inline") {
		t.Fatalf("expected inline disposition param, got: %s", url)
	}
	if !strings.Contains(url, "q-header-list=host") {
		t.Fatalf("expected host in signed header list, got: %s", url)
	}
	if strings.Contains(url, "myqcloud.com") {
		t.Fatalf("default domain must not appear, got: %s", url)
	}
}

// TestPresignDefaultDomain verifies the original behaviour is untouched when no
// custom domain is configured: the presigned URL keeps the default bucket host.
func TestPresignDefaultDomain(t *testing.T) {
	svc, err := newCosClient("weknora-daily-1313344759", "ap-seoul", "AKIDtestsecretid", "testsecretkey")
	if err != nil {
		t.Fatalf("newCosClient failed: %v", err)
	}
	svc.cosPathPrefix = "weknora"

	url, err := svc.GetFileURL(context.Background(), "cos://weknora-daily-1313344759/ap-seoul/weknora/10000/abc.pdf")
	if err != nil {
		t.Fatalf("GetFileURL failed: %v", err)
	}
	if !strings.HasPrefix(url, "https://weknora-daily-1313344759.cos.ap-seoul.myqcloud.com/weknora/10000/abc.pdf?") {
		t.Fatalf("expected default-domain presigned URL, got: %s", url)
	}
}
