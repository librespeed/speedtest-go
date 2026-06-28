package results

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/rand"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/render"
	"github.com/librespeed/speedtest-go/config"
	"github.com/librespeed/speedtest-go/database"
	"github.com/librespeed/speedtest-go/database/schema"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/oklog/ulid/v2"
	log "github.com/sirupsen/logrus"
	"golang.org/x/image/font"
)

const (
	watermark = "LibreSpeed"

	labelMS       = " ms"
	labelMbps     = "Mbit/s"
	labelPing     = "Ping"
	labelJitter   = "Jitter"
	labelDownload = "Download"
	labelUpload   = "Upload"
)

//go:embed fonts/NotoSansDisplay-Medium.ttf
var fontMediumBytes []byte

//go:embed fonts/NotoSansDisplay-Light.ttf
var fontLightBytes []byte

var (
	ipv4Regex     = regexp.MustCompile(`(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)`)
	ipv6Regex     = regexp.MustCompile(`(([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]{1,}|::(ffff(:0{1,4})?:)?((25[0-5]|(2[0-4]|1?[0-9])?[0-9])\.){3}(25[0-5]|(2[0-4]|1?[0-9])?[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1?[0-9])?[0-9])\.){3}(25[0-5]|(2[0-4]|1?[0-9])?[0-9]))`)
	hostnameRegex = regexp.MustCompile(`"hostname":"([^\\\\"]|\\\\")*"`)

	fontLight, fontBold                                                                                                *truetype.Font
	pingJitterLabelFace, upDownLabelFace, pingJitterValueFace, upDownValueFace, smallLabelFace, ispFace, watermarkFace font.Face
	valueFace, labelFace, unitFace, footerFace                                                                         font.Face

	canvasWidth, canvasHeight = 900, 240
	dpi                       = 150.0

	// rgba(255,255,255,0.03) over #07090f → card bg matching HTML .r-card
	// rgba(255,255,255,0.07) over #07090f → border matching HTML --border
	colorBg       = color.RGBA{7, 9, 15, 255}
	colorCard     = color.RGBA{14, 16, 22, 255}
	colorBorder   = color.RGBA{24, 26, 32, 255}
	colorText     = color.RGBA{238, 242, 255, 255}
	colorMuted    = color.RGBA{105, 110, 130, 255}
	colorDim      = color.RGBA{52, 56, 72, 255}
	colorDownload = color.RGBA{34, 211, 238, 255}
	colorUpload   = color.RGBA{167, 139, 250, 255}
	colorPing     = color.RGBA{52, 211, 153, 255}
	colorJitter   = color.RGBA{251, 191, 36, 255}
	colorLossWarn = color.RGBA{251, 191, 36, 255}  // same as jitter: yellow for 0<loss<2%
	colorLossBad  = color.RGBA{248, 113, 113, 255} // #f87171: red for loss>=2%
)

type Result struct {
	ProcessedString string         `json:"processedString"`
	RawISPInfo      IPInfoResponse `json:"rawIspInfo"`
}

type IPInfoResponse struct {
	IP           string `json:"ip"`
	Hostname     string `json:"hostname"`
	City         string `json:"city"`
	Region       string `json:"region"`
	Country      string `json:"country"`
	Location     string `json:"loc"`
	Organization string `json:"org"`
	Postal       string `json:"postal"`
	Timezone     string `json:"timezone"`
	Readme       string `json:"readme"`
}

func Initialize(c *config.Config) {
	// changed to use Noto Sans instead of OpenSans, due to issue:
	// https://github.com/golang/freetype/issues/8
	fLight, err := freetype.ParseFont(fontLightBytes)
	if err != nil {
		log.Fatalf("Error parsing NotoSansDisplay-Light font: %s", err)
	}
	fontLight = fLight

	fMedium, err := freetype.ParseFont(fontMediumBytes)
	if err != nil {
		log.Fatalf("Error parsing NotoSansDisplay-Medium font: %s", err)
	}
	fontBold = fMedium

	pingJitterLabelFace = truetype.NewFace(fontBold, &truetype.Options{
		Size:    12,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})

	upDownLabelFace = truetype.NewFace(fontBold, &truetype.Options{
		Size:    14,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})

	pingJitterValueFace = truetype.NewFace(fontLight, &truetype.Options{
		Size:    16,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})

	upDownValueFace = truetype.NewFace(fontLight, &truetype.Options{
		Size:    18,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})

	smallLabelFace = truetype.NewFace(fontBold, &truetype.Options{
		Size:    10,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})

	ispFace = truetype.NewFace(fontBold, &truetype.Options{
		Size:    8,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})

	watermarkFace = truetype.NewFace(fontLight, &truetype.Options{
		Size:    6,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})

	valueFace = truetype.NewFace(fontLight, &truetype.Options{
		Size:    30,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
	labelFace = truetype.NewFace(fontBold, &truetype.Options{
		Size:    7.5,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
	unitFace = truetype.NewFace(fontBold, &truetype.Options{
		Size:    8.5,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
	footerFace = truetype.NewFace(fontBold, &truetype.Options{
		Size:    7,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
}

func Record(w http.ResponseWriter, r *http.Request) {
	conf := config.LoadedConfig()
	if conf.DatabaseType == "none" {
		render.PlainText(w, r, "Telemetry is disabled")
		return
	}

	ipAddr, _, _ := net.SplitHostPort(r.RemoteAddr)
	userAgent := r.UserAgent()
	language := r.Header.Get("Accept-Language")

	ispInfo := r.FormValue("ispinfo")
	download := r.FormValue("dl")
	upload := r.FormValue("ul")
	ping := r.FormValue("ping")
	jitter := r.FormValue("jitter")
	logs := r.FormValue("log")
	extra := r.FormValue("extra")

	if config.LoadedConfig().RedactIP {
		ipAddr  = "0.0.0.0"
		ispInfo = ipv4Regex.ReplaceAllString(ispInfo, "0.0.0.0")
		logs    = ipv4Regex.ReplaceAllString(logs, "0.0.0.0")
		ispInfo = ipv6Regex.ReplaceAllString(ispInfo, "::")
		logs    = ipv6Regex.ReplaceAllString(logs, "::")
		ispInfo = hostnameRegex.ReplaceAllString(ispInfo, `"hostname":"REDACTED"`)
		logs    = hostnameRegex.ReplaceAllString(logs, `"hostname":"REDACTED"`)
	}

	var record schema.TelemetryData
	record.IPAddress = ipAddr
	if ispInfo == "" {
		record.ISPInfo = "{}"
	} else {
		record.ISPInfo = ispInfo
	}
	record.Extra = extra
	record.UserAgent = userAgent
	record.Language = language
	record.Download = download
	record.Upload = upload
	record.Ping = ping
	record.Jitter = jitter
	record.Log = logs
	record.ClientID = r.FormValue("client_id")

	t := time.Now()
	entropy := ulid.Monotonic(rand.New(rand.NewSource(t.UnixNano())), 0)
	uuid := ulid.MustNew(ulid.Timestamp(t), entropy)
	record.UUID = uuid.String()

	err := database.DB.Insert(&record)
	if err != nil {
		log.Errorf("Error inserting into database: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	responseID := uuid.String()
	if config.LoadedConfig().EnableIDObfuscation {
		responseID = ObfuscateULID(uuid.String())
	}

	if _, err := w.Write([]byte("id " + responseID)); err != nil {
		log.Errorf("Error writing ID to telemetry request: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func fillRect(canvas *image.RGBA, x, y, w, h int, c color.Color) {
	for row := y; row < y+h; row++ {
		for col := x; col < x+w; col++ {
			canvas.Set(col, row, c)
		}
	}
}

// fillRoundedRect draws a filled rectangle with rounded corners (radius r).
// Matches CSS border-radius: uses a circle-distance test at each corner.
func fillRoundedRect(canvas *image.RGBA, x, y, w, h, r int, c color.Color) {
	// inner rect whose corners are the circle centers
	x1, y1 := x+r, y+r
	x2, y2 := x+w-1-r, y+h-1-r
	for row := y; row < y+h; row++ {
		for col := x; col < x+w; col++ {
			nearX := col
			if nearX < x1 {
				nearX = x1
			} else if nearX > x2 {
				nearX = x2
			}
			nearY := row
			if nearY < y1 {
				nearY = y1
			} else if nearY > y2 {
				nearY = y2
			}
			dx, dy := col-nearX, row-nearY
			if dx*dx+dy*dy <= r*r {
				canvas.Set(col, row, c)
			}
		}
	}
}

// drawText draws centered text and returns the drawn width in pixels.
func drawText(drawer *font.Drawer, text string, cx, y int, src image.Image, face font.Face) {
	drawer.Face = face
	drawer.Src = src
	w := drawer.MeasureString(text).Round()
	drawer.Dot = freetype.Pt(cx-w/2, y)
	drawer.DrawString(text)
}

func DrawPNG(w http.ResponseWriter, r *http.Request) {
	conf := config.LoadedConfig()
	if conf.DatabaseType == "none" {
		return
	}

	rawID := r.FormValue("id")
	uuid := ResolveID(rawID)
	record, err := database.DB.FetchByUUID(uuid)
	if err != nil {
		log.Errorf("Error querying database: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var result Result
	if err := json.Unmarshal([]byte(record.ISPInfo), &result); err != nil {
		log.Errorf("Error parsing ISP info: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	canvas := image.NewRGBA(image.Rect(0, 0, canvasWidth, canvasHeight))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(colorBg), image.Point{}, draw.Src)
	drawer := &font.Drawer{Dst: canvas}

	// Layout — mirrors HTML .r-card grid
	const (
		sidePad = 20
		barH    = 28
		radius  = 14 // matches CSS border-radius: 1rem
	)

	// Determine layout: 5 cards when loss param is present, 4 otherwise
	lossParam := strings.TrimSpace(r.FormValue("loss"))
	numCards  := 4
	gapW      := 24
	if lossParam != "" {
		numCards = 5
		gapW     = 10 // tighter gap to keep 900px canvas with 5 cards
	}

	colW    := (canvasWidth - sidePad*2 - gapW*(numCards-1)) / numCards
	barY    := canvasHeight - barH
	cardTop := sidePad
	cardH   := barY - cardTop

	// Center content block (88px) in card
	blockTop := cardTop + (cardH-88)/2
	valueY   := blockTop + 45
	labelY   := valueY + 21
	unitY    := labelY + 19

	// Determine packet-loss color from value
	lossColor := colorPing // green: 0%
	if lossParam != "" {
		lf, _ := strconv.ParseFloat(lossParam, 64)
		if lf > 0 && lf < 2 { lossColor = colorLossWarn }
		if lf >= 2           { lossColor = colorLossBad  }
	}

	type metric struct {
		label, value, unit string
		col                color.RGBA
	}
	metrics := []metric{
		{"DOWNLOAD", formatSpeed(record.Download),   "Mbps", colorDownload},
		{"UPLOAD",   formatSpeed(record.Upload),     "Mbps", colorUpload},
		{"PING",     formatLatency(record.Ping),     "ms",   colorPing},
		{"JITTER",   formatLatency(record.Jitter),   "ms",   colorJitter},
	}
	if lossParam != "" {
		metrics = append(metrics, metric{"LOSS", formatLoss(lossParam), "%", lossColor})
	}

	for i, m := range metrics {
		x := sidePad + i*(colW+gapW)

		// border + card background — matches HTML .r-card
		fillRoundedRect(canvas, x-1, cardTop-1, colW+2, cardH+2, radius+1, colorBorder)
		fillRoundedRect(canvas, x, cardTop, colW, cardH, radius, colorCard)

		cx := x + colW/2
		drawText(drawer, m.value, cx, valueY, image.NewUniform(m.col),      valueFace)
		drawText(drawer, m.label, cx, labelY, image.NewUniform(colorMuted),  labelFace)
		drawText(drawer, m.unit,  cx, unitY,  image.NewUniform(colorDim),   unitFace)
	}

	// thin footer bar
	fillRect(canvas, 0, barY, canvasWidth, 1, colorBorder)
	footerY := barY + 19

	isp := extractISP(result.ProcessedString)
	if isp != "" {
		drawer.Face = footerFace
		drawer.Src = image.NewUniform(colorMuted)
		drawer.Dot = freetype.Pt(sidePad, footerY)
		drawer.DrawString(isp)
	}

	ts := record.Timestamp.Format("2006-01-02 15:04 UTC")
	drawer.Face = footerFace
	drawer.Src = image.NewUniform(colorMuted)
	tsW := drawer.MeasureString(ts)
	drawer.Dot = freetype.Pt(canvasWidth/2-tsW.Round()/2, footerY)
	drawer.DrawString(ts)

	drawer.Face = footerFace
	drawer.Src = image.NewUniform(colorMuted)
	wmW := drawer.MeasureString(watermark)
	drawer.Dot = freetype.Pt(canvasWidth-wmW.Round()-sidePad, footerY)
	drawer.DrawString(watermark)

	w.Header().Set("Content-Disposition", "inline; filename="+uuid+".png")
	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, canvas); err != nil {
		log.Errorf("Failed to output image to HTTP client: %s", err)
	}
}

func formatSpeed(s string) string {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if f >= 1000 {
		return fmt.Sprintf("%.0f", f)
	}
	if f >= 10 {
		return fmt.Sprintf("%.1f", f)
	}
	return fmt.Sprintf("%.2f", f)
}

func formatLoss(s string) string {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if f == 0 {
		return "0.0"
	}
	if f < 10 {
		return fmt.Sprintf("%.1f", f)
	}
	return fmt.Sprintf("%.0f", f)
}

func formatLatency(s string) string {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if f < 10 {
		return fmt.Sprintf("%.1f", f)
	}
	return fmt.Sprintf("%.0f", f)
}

func extractISP(processed string) string {
	if strings.Contains(processed, "-") {
		parts := strings.SplitN(processed, "-", 2)
		p := parts[1]
		if i := strings.Index(p, "("); i >= 0 {
			p = p[:i]
		}
		if isp := strings.TrimSpace(p); isp != "" {
			return "ISP: " + isp
		}
	}
	return ""
}
