package home

import (
	"net/url"
	"strings"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/internal/fn"
)

func renderBookmarkHref(sourceURL string, localURL string, preferLocal bool, enableEncryptedLink bool, requestURL *fn.DynamicURL) string {
	if shouldUseLocalRedirect(sourceURL, localURL, preferLocal, requestURL) {
		return define.MiscPages.RedirLocal.Path + "?go=" + data.Base64EncodeUrl(sourceURL) + "&local=" + data.Base64EncodeUrl(localURL)
	}
	if strings.HasPrefix(sourceURL, "chrome-extension://") || enableEncryptedLink {
		return define.MiscPages.RedirHelper.Path + "?go=" + data.Base64EncodeUrl(sourceURL)
	}
	return sourceURL
}

func shouldUseLocalRedirect(sourceURL string, localURL string, preferLocal bool, requestURL *fn.DynamicURL) bool {
	if !preferLocal || !isHTTPBookmarkURL(sourceURL) || !isHTTPBookmarkURL(localURL) {
		return false
	}
	if requestURL == nil || strings.TrimSpace(requestURL.Hostname) == "" {
		return true
	}
	return fn.LocalURLMayShareNetworkWithRequest(requestURL, localURL)
}

func isHTTPBookmarkURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != ""
}
