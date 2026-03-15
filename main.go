package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"unicode/utf8"

	"text/template"

	"github.com/labstack/echo/v4"
	"github.com/shkh/lastfm-go/lastfm"
)

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

type Playing struct {
	Url           string
	Icon          string
	Title         string
	Status        string
	Animate       bool
	Width         string
	Height        string
	ScrollPercent float64
}

type EmbedCode struct {
	LinkUrl  string
	ImageUrl string
}

func ToDataUrl(imgUrl string) (string, error) {
	resp, err := http.Get(imgUrl)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	w := bytes.NewBuffer(make([]byte, 0, 1024))

	w.WriteString("data:" + resp.Header.Get("Content-Type") + ";base64,")

	enc := base64.NewEncoder(base64.StdEncoding, w)
	defer enc.Close()

	io.Copy(enc, resp.Body)

	return w.String(), nil
}

func main() {
	imageSize := os.Getenv("IMAGE_SIZE")

	fm := lastfm.New(os.Getenv("LASTFM_KEY"), os.Getenv("LASTFM_SECRET"))
	e := echo.New()

	e.Renderer = &Template{
		templates: template.Must(template.ParseGlob("views/*")),
	}

	e.HTTPErrorHandler = func(err error, c echo.Context) {
		e.Logger.Error(err)
	}

	e.GET("/", func(c echo.Context) error {
		return c.Render(http.StatusOK, "index.html", nil)
	})

	e.GET("/embed_code", func(c echo.Context) error {
		user := c.QueryParam("user")

		req := c.Request()

		var host string
		if req.Header.Get("X-Forwarded-Proto") == "https" {
			host = "https://"
		} else {
			host = "http://"
		}

		host += req.Host

		link, err := url.JoinPath(host, "/playing/", user, "/url")
		if err != nil {
			return err
		}

		imageUrl, err := url.JoinPath(host, "/playing/", user)
		if err != nil {
			return err
		}

		return c.Render(http.StatusOK, "embed.html", EmbedCode{
			LinkUrl:  link,
			ImageUrl: imageUrl,
		})
	})

	e.GET("/playing/:user/url", func(c echo.Context) error {
		res, err := fm.User.GetRecentTracks(lastfm.P{
			"user": c.Param("user"),
		})
		if err != nil {
			return err
		}

		track := res.Tracks[0]

		return c.Redirect(http.StatusFound, track.Url)
	})

	e.GET("/playing/:user", func(c echo.Context) error {
		c.Response().Header().Set("Content-Type", "image/svg+xml")

		res, err := fm.User.GetRecentTracks(lastfm.P{
			"user": c.Param("user"),
		})
		if err != nil {
			return err
		}

		track := res.Tracks[0]

		var url string
		if len(track.Images) > 0 {
			url = track.Images[0].Url
		}
		for _, i := range track.Images {
			if i.Size == imageSize {
				url = i.Url
				break
			}
		}

		var icon string
		if url != "" {
			icon, err = ToDataUrl(url)
			if err != nil {
				return err
			}
		}

		header := c.Response().Header()
		header.Set("Cache-Control", "no-cache, no-store, must-revalidate, max-age=0")
		header.Set("CDN-Cache-Control", "no-cache")
		header.Set("Cloudflare-CDN-Cache-Control", "no-cache")

		const (
			fontSize       = 2.1607
			charWidthRatio = 0.60
			containerWidth = 22.225
			viewportWidth  = 29.632
		)
		runeCount := utf8.RuneCountInString(track.Name)
		estTextWidth := float64(runeCount) * fontSize * charWidthRatio
		overflow := estTextWidth - containerWidth
		animate := overflow > 0
		scrollPercent := math.Max(0, overflow) / viewportWidth * 100

		// Get scale from query parameter (default: 1.0)
		scale := 1.0
		if scaleParam := c.QueryParam("scale"); scaleParam != "" {
			if s, err := strconv.ParseFloat(scaleParam, 64); err == nil && s > 0 && s <= 10 {
				scale = s
			}
		}

		// Base dimensions in mm
		baseWidth := 29.632
		baseHeight := 7.4083

		return c.Render(http.StatusOK, "playing.svg", Playing{
			Url:           track.Url,
			Icon:          icon,
			Title:         track.Name,
			Status:        track.Artist.Name,
			Animate:       animate,
			Width:         fmt.Sprintf("%.3fmm", baseWidth*scale),
			Height:        fmt.Sprintf("%.3fmm", baseHeight*scale),
			ScrollPercent: scrollPercent,
		})
	})

	addr := os.Getenv("PORT")
	if addr == "" {
		addr = ":8080"
	}

	e.Logger.Fatal(e.Start(addr))
}
