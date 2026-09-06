package browserfixture_test

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func TestFixtureChooserStandardsDocument(t *testing.T) {
	f := newFixture(t, "")
	response, err := http.Get(f.Server.URL + "/_fixture/")
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "text/html; charset=utf-8", response.Header.Get("Content-Type"))
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	require.Empty(t, response.Header.Get("Set-Cookie"))
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(body), "<!DOCTYPE html>"), "HTML5 doctype must precede the document, not be injected into its body")
	document, err := html.Parse(bytes.NewReader(body))
	require.NoError(t, err)
	require.NotNil(t, document.FirstChild)
	require.Equal(t, html.DoctypeNode, document.FirstChild.Type)
	require.Equal(t, "html", document.FirstChild.Data)
	require.Empty(t, document.FirstChild.Attr, "no legacy public/system doctype that could select quirks mode")
	var head, bodyNode, icon, form *html.Node
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "head":
				head = n
			case "body":
				bodyNode = n
			case "link":
				if htmlAttribute(n, "rel") == "icon" {
					icon = n
				}
			case "form":
				form = n
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(document)
	require.NotNil(t, head)
	require.NotNil(t, bodyNode)
	require.NotNil(t, icon)
	require.Same(t, head, icon.Parent)
	require.Equal(t, "/favicon.ico", htmlAttribute(icon, "href"))
	require.Equal(t, "image/vnd.microsoft.icon", htmlAttribute(icon, "type"))
	require.Equal(t, "32x32", htmlAttribute(icon, "sizes"))
	require.NotNil(t, form)
	require.Equal(t, "/_fixture/session", htmlAttribute(form, "action"))
	require.Equal(t, "post", htmlAttribute(form, "method"))
}

func htmlAttribute(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func TestFixtureFaviconDecodesWithoutSession(t *testing.T) {
	f := newFixture(t, "")
	response, err := http.Get(f.Server.URL + "/favicon.ico")
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "image/vnd.microsoft.icon", response.Header.Get("Content-Type"))
	require.Equal(t, "nosniff", response.Header.Get("X-Content-Type-Options"))
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	require.Empty(t, response.Header.Get("Set-Cookie"))
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, fixtureFavicon, body)
	require.Equal(t, strconv.Itoa(len(body)), response.Header.Get("Content-Length"))
	// Validate the ICO directory and actually decode its PNG image. An empty
	// success response, HTML fallback, hidden sprite, or malformed icon fails.
	require.Greater(t, len(body), 22)
	require.Equal(t, uint16(0), binary.LittleEndian.Uint16(body[0:2]))
	require.Equal(t, uint16(1), binary.LittleEndian.Uint16(body[2:4]))
	require.Equal(t, uint16(1), binary.LittleEndian.Uint16(body[4:6]))
	require.Equal(t, byte(32), body[6])
	require.Equal(t, byte(32), body[7])
	require.Equal(t, uint16(1), binary.LittleEndian.Uint16(body[10:12]))
	require.Equal(t, uint16(32), binary.LittleEndian.Uint16(body[12:14]))
	size := int(binary.LittleEndian.Uint32(body[14:18]))
	offset := int(binary.LittleEndian.Uint32(body[18:22]))
	require.Equal(t, 22, offset)
	require.Equal(t, len(body)-offset, size)
	image, err := png.Decode(bytes.NewReader(body[offset:]))
	require.NoError(t, err)
	require.Equal(t, 32, image.Bounds().Dx())
	require.Equal(t, 32, image.Bounds().Dy())
	visible := 0
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			_, _, _, alpha := image.At(x, y).RGBA()
			if alpha != 0 {
				visible++
			}
		}
	}
	require.Positive(t, visible, "icon must contain visible artwork")

	head, err := http.Head(f.Server.URL + "/favicon.ico")
	require.NoError(t, err)
	defer head.Body.Close()
	require.Equal(t, http.StatusOK, head.StatusCode)
	require.Equal(t, response.Header.Get("Content-Type"), head.Header.Get("Content-Type"))
	require.Equal(t, response.Header.Get("Content-Length"), head.Header.Get("Content-Length"))
	empty, err := io.ReadAll(head.Body)
	require.NoError(t, err)
	require.Empty(t, empty, "HEAD has metadata only; GET above must return decodable bytes")
}

func TestFixtureFaviconDoesNotBroadenRoutes(t *testing.T) {
	f := newFixture(t, t.TempDir())
	for path, status := range map[string]int{
		"/favicon.ico/missing":   http.StatusNotFound,
		"/_fixture/favicon.ico":  http.StatusNotFound,
		"/missing.ico":           http.StatusNotFound,
		"/studio/v1/auth/me":     http.StatusUnauthorized,
		"/studio/v1/favicon.ico": http.StatusUnauthorized,
	} {
		response, err := http.Get(f.Server.URL + path)
		require.NoError(t, err)
		response.Body.Close()
		require.Equal(t, status, response.StatusCode, path)
		require.NotEqual(t, "image/vnd.microsoft.icon", response.Header.Get("Content-Type"), path)
	}
	req, err := http.NewRequest(http.MethodPost, f.Server.URL+"/favicon.ico", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", f.Server.URL)
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusMethodNotAllowed, response.StatusCode)
	require.Equal(t, "GET, HEAD", response.Header.Get("Allow"))
}
