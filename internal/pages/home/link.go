package home

import (
	"net/url"
	"strings"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
)

func renderBookmarkHref(sourceURL string, localURL string, preferLocal bool, enableEncryptedLink bool) string {
	if preferLocal && isHTTPBookmarkURL(sourceURL) && isHTTPBookmarkURL(localURL) {
		return define.MiscPages.RedirLocal.Path + "?go=" + data.Base64EncodeUrl(sourceURL) + "&local=" + data.Base64EncodeUrl(localURL)
	}
	if strings.HasPrefix(sourceURL, "chrome-extension://") || enableEncryptedLink {
		return define.MiscPages.RedirHelper.Path + "?go=" + data.Base64EncodeUrl(sourceURL)
	}
	return sourceURL
}

func isHTTPBookmarkURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != ""
}
