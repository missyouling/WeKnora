package file

import "testing"

func TestRewriteHost(t *testing.T) {
	svc := &cosFileService{customDomain: "cos.mozuiapp.com"}
	base := "https://weknora-daily-1313344759.cos.ap-seoul.myqcloud.com/"
	raw := "https://weknora-daily-1313344759.cos.ap-seoul.myqcloud.com/weknora/10000/x.pdf?q-sign-algorithm=sha1&q-sign-time=1;2&response-content-disposition=inline"
	got := svc.rewriteHost(raw, base)
	want := "https://cos.mozuiapp.com/weknora/10000/x.pdf?q-sign-algorithm=sha1&q-sign-time=1;2&response-content-disposition=inline"
	if got != want {
		t.Fatalf("rewriteHost mismatch:\n got=%q\nwant=%q", got, want)
	}

	// no custom domain -> unchanged
	plain := &cosFileService{customDomain: ""}
	if got := plain.rewriteHost(raw, base); got != raw {
		t.Fatalf("no-custom-domain should not rewrite: %q", got)
	}

	// http scheme base
	httpBase := "http://weknora-daily-1313344759.cos.ap-seoul.myqcloud.com/"
	rawHTTP := "http://weknora-daily-1313344759.cos.ap-seoul.myqcloud.com/x.pdf"
	if got := svc.rewriteHost(rawHTTP, httpBase); got != "http://cos.mozuiapp.com/x.pdf" {
		t.Fatalf("http scheme rewrite failed: %q", got)
	}
}
